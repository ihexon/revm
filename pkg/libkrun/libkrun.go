//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L ../../out/lib/ -lkrun.1.15.1 -lkrunfw.4
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// GoStringList2CStringArray takes an array of Go strings and converts it to an array of CStrings.
// The function returned should be deferred by the caller to free allocated memory.
func GoStringList2CStringArray(stringList []string) ([]*C.char, func()) {
	list := make([]*C.char, len(stringList)+1)
	for i, str := range stringList {
		list[i] = C.CString(str)
	}

	return list, func() {
		for _, str := range list {
			if str != nil {
				C.free(unsafe.Pointer(str))
			}
		}
	}
}

func GoString2CString(str string) (*C.char, func()) {
	cStr := C.CString(str)
	return cStr, func() {
		if cStr != nil {
			C.free(unsafe.Pointer(cStr))
		}
	}
}

type Stubber struct {
	krunCtxID uint32
	vmc       *vmconfig.VMConfig
}

func NewStubber(vmc *vmconfig.VMConfig) *Stubber {
	return &Stubber{
		vmc: vmc,
	}
}

func (v *Stubber) Create(ctx context.Context) error {
	id := C.krun_create_ctx()
	if id < 0 {
		return fmt.Errorf("failed to create vm ctx id, return %v", id)
	}
	v.krunCtxID = uint32(id)
	logrus.Debugf("created vm ctx id: %d", v.krunCtxID)

	logrus.Debugf("set libkrun log level to info")
	if ret := C.krun_set_log_level(C.KRUN_LOG_LEVEL_INFO); ret != 0 {
		return fmt.Errorf("failed to set log level, return %v", ret)
	}

	logrus.Infof("set vm memory: %d MB, cpu: %d", v.vmc.MemoryInMB, v.vmc.Cpus)
	if ret := C.krun_set_vm_config(C.uint32_t(v.krunCtxID), C.uint8_t(v.vmc.Cpus), C.uint32_t(v.vmc.MemoryInMB)); ret != 0 {
		return fmt.Errorf("failed to set vm config, return %v", ret)
	}

	if v.vmc.RunMode == define.ContainerMode.String() || v.vmc.RunMode == define.RootFsMode.String() {
		logrus.Debugf("vm run mode is rootfs mode")
		if err := v.setRootFS(); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("libkrun do not support run mode: %q", v.vmc.RunMode)
	}

	if err := v.setGPU(); err != nil {
		return err
	}

	if err := v.setRLimited(); err != nil {
		return err
	}

	if err := v.setNetworkProvider(); err != nil {
		return err
	}
	if err := v.addRawDisk(ctx); err != nil {
		return err
	}

	if err := v.addVirtioFS(); err != nil {
		return err
	}

	if err := v.NestVirt(ctx); err != nil {
		return err
	}

	if err := v.addVSockListenInHost(ctx, v.krunCtxID); err != nil {
		return err
	}

	return nil
}

func (v *Stubber) Start(ctx context.Context) error {
	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set rlimit: %v", err)
	}

	if err := v.setCommandLine(v.vmc.Cmdline.Workspace, v.vmc.Cmdline.Env); err != nil {
		return err
	}

	return execCmdlineInVM(ctx, v.krunCtxID)
}

func (v *Stubber) Stop(ctx context.Context) error {
	return stopVM(ctx, v.krunCtxID)
}

func (v *Stubber) setNetworkProvider() error {
	parse, err := url.Parse(v.vmc.NetworkStackBackend)
	if err != nil {
		return fmt.Errorf("failed to parse network stack backend: %w", err)
	}

	gvpSocket, fn1 := GoString2CString(parse.Path)
	defer fn1()

	logrus.Infof("set vm network backend: %q", parse.Path)
	if ret := C.krun_set_gvproxy_path(C.uint32_t(v.krunCtxID), gvpSocket); ret != 0 {
		return fmt.Errorf("failed to set gvproxy path, return %v", ret)
	}

	return nil
}

func (v *Stubber) GetVMConfigure() (*vmconfig.VMConfig, error) {
	if v.vmc == nil {
		return nil, fmt.Errorf("can not get vm config object, vmconfig is nil")
	}
	return v.vmc, nil
}

const (
	RLIMIT_NPROC = "6"
	SoftLimit    = "4096"
	HardLimit    = "8192"
)

func (v *Stubber) setRLimited() error {
	str := fmt.Sprintf("%s=%s:%s", RLIMIT_NPROC, SoftLimit, HardLimit)
	limitStr, fn1 := GoStringList2CStringArray(
		[]string{str},
	)
	defer fn1()

	logrus.Debugf("set vm rlimit: %q", str)
	if ret := C.krun_set_rlimits(C.uint32_t(v.krunCtxID), &limitStr[0]); ret != 0 {
		return fmt.Errorf("failed to set rlimits, return %v", ret)
	}
	return nil
}

func (v *Stubber) setCommandLine(dir string, env []string) error {
	cleanUp := system.CleanUp()
	defer cleanUp.DoClean()

	workdir, fn1 := GoString2CString(dir)
	cleanUp.Add(func() error {
		fn1()
		return nil
	})

	logrus.Debugf("set vm workdir: %q", dir)
	if ret := C.krun_set_workdir(C.uint32_t(v.krunCtxID), workdir); ret != 0 {
		return fmt.Errorf("failed to set workdir, return %v", ret)
	}

	logrus.Debugf("guest bootstrap is: %q", v.vmc.Cmdline.GuestAgent)
	cBootstrapBinPath, fn2 := GoString2CString(v.vmc.Cmdline.GuestAgent)
	cleanUp.Add(func() error {
		fn2()
		return nil
	})

	logrus.Debugf("guest bootstrap args is: %q", v.vmc.Cmdline.GuestAgentArgs)
	cBootstrapBinArgs, fn3 := GoStringList2CStringArray(v.vmc.Cmdline.GuestAgentArgs)
	cleanUp.Add(func() error {
		fn3()
		return nil
	})

	logrus.Infof("pass env to guest: %q", env)
	cEnvPassIn, fn4 := GoStringList2CStringArray(env)
	cleanUp.Add(func() error {
		fn4()
		return nil
	})

	if ret := C.krun_set_exec(C.uint32_t(v.krunCtxID), cBootstrapBinPath, &cBootstrapBinArgs[0], &cEnvPassIn[0]); ret != 0 {
		return fmt.Errorf("failed to set exec, return %v", ret)
	}

	return nil
}

const (
	VIRGLRENDERER_USE_EGL            = 1 << 0
	VIRGLRENDERER_THREAD_SYNC        = 1 << 1
	VIRGLRENDERER_USE_GLX            = 1 << 2
	VIRGLRENDERER_USE_SURFACELESS    = 1 << 3
	VIRGLRENDERER_USE_GLES           = 1 << 4
	VIRGLRENDERER_USE_EXTERNAL_BLOB  = 1 << 5
	VIRGLRENDERER_VENUS              = 1 << 6
	VIRGLRENDERER_NO_VIRGL           = 1 << 7
	VIRGLRENDERER_USE_ASYNC_FENCE_CB = 1 << 8
	VIRGLRENDERER_RENDER_SERVER      = 1 << 9
	VIRGLRENDERER_DRM                = 1 << 10
)

func (v *Stubber) setGPU() error {
	logrus.Debugf("set gpu with %q", "VIRGLRENDERER_VENUS|VIRGLRENDERER_NO_VIRGL")
	if err := C.krun_set_gpu_options(C.uint32_t(v.krunCtxID), C.uint32_t(VIRGLRENDERER_VENUS|VIRGLRENDERER_NO_VIRGL)); err != 0 {
		return fmt.Errorf("failed to set gpu options,return %v", err)
	}
	return nil
}

func (v *Stubber) setRootFS() error {
	rootfs, fn1 := GoString2CString(v.vmc.RootFS)
	defer fn1()

	if ret := C.krun_set_root(C.uint32_t(v.krunCtxID), rootfs); ret != 0 {
		return fmt.Errorf("failed to set root, return %v", ret)
	}
	return nil
}

func (v *Stubber) addRawDisk(ctx context.Context) error {
	for _, disk := range v.vmc.BlkDevs {
		if err := addRawDisk(v.krunCtxID, disk.Path); err != nil {
			return err
		}
	}

	return nil
}

func (v *Stubber) addVirtioFS() error {
	for _, mount := range v.vmc.Mounts {
		if err := addVirtioFS(v.krunCtxID, mount.Tag, mount.Source); err != nil {
			return fmt.Errorf("failed to add virtiofs: %w", err)
		}
	}

	return nil
}

// StartNetwork start gvproxy in go routine for guest network
func (v *Stubber) StartNetwork(ctx context.Context) error {
	return gvproxy.Run(ctx, v.vmc)
}

func (v *Stubber) NestVirt(ctx context.Context) error {
	ret := C.krun_check_nested_virt()

	switch ret {
	case 0:
		logrus.Infof("current system not support nest virtualization, skip enable nested virtuallization")
		return nil
	case 1:
		logrus.Infof("current system support nested virtuallization")
	default:
		return fmt.Errorf("failed to check nested virtuallization support, return %v", ret)
	}

	if ret := C.krun_set_nested_virt(C.uint32_t(v.krunCtxID), true); ret != 0 {
		return fmt.Errorf("nested virtuallization support, but enable nested virtuallization failed")
	}

	logrus.Infof("enable nested virtualization successful")

	return nil
}

func (v *Stubber) SetKernel(ctx context.Context) error {
	return setKernel(ctx, v.krunCtxID, v.vmc.Kernel, v.vmc.Initrd, v.vmc.KernelCmdline...)
}

func stopVM(tx context.Context, vmID uint32) error {
	return nil
}

func addRawDisk(ctxID uint32, disk string) error {
	if _, err := os.Stat(disk); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("access raw disk %q with err: %w", disk, err)
	}

	blockID, fn1 := GoString2CString(uuid.New().String())
	defer fn1()

	extDisk, fn2 := GoString2CString(disk)
	defer fn2()

	logrus.Infof("add raw disk %q to vm", disk)
	if ret := C.krun_add_disk2(C.uint32_t(ctxID), blockID, extDisk, C.KRUN_DISK_FORMAT_RAW, false); ret != 0 {
		return fmt.Errorf("failed to add disk, return %v", ret)
	}

	return nil
}

func addVirtioFS(ctxID uint32, tag, path string) error {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	hostPath, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return fmt.Errorf("failed to eval symlinks: %w", err)
	}

	if !system.IsPathExist(hostPath) {
		return fmt.Errorf("host dir %q not exist", hostPath)
	}

	cHostPath, fn1 := GoString2CString(hostPath)
	defer fn1()

	cTag, fn2 := GoString2CString(tag)
	defer fn2()

	logrus.Infof("add virtiofs %q to vm", hostPath)
	if ret := C.krun_add_virtiofs2(C.uint32_t(ctxID), cTag, cHostPath, C.uint64_t(1<<29)); ret != 0 {
		return fmt.Errorf("failed to add virtiofs, return: %v", ret)
	}

	return nil
}

func (v *Stubber) addVSockListenInHost(ctx context.Context, vmCtxID uint32) error {
	return addVSockListenInHost(ctx, vmCtxID, uint32(define.DefaultVSockPort), v.vmc.IgnProvisionerAddr)
}

func addVSockListenInHost(ctx context.Context, vmCtxID uint32, vsockPort uint32, ignServerAddr string) error {
	addr, err := network.ParseUnixAddr(ignServerAddr)
	if err != nil {
		return fmt.Errorf("failed to parse unix addr: %w", err)
	}

	cIgnServerListenPath, fn1 := GoString2CString(addr.Path)
	defer fn1()

	if ret := C.krun_add_vsock_port2(C.uint32_t(vmCtxID), C.uint32_t(vsockPort), cIgnServerListenPath, false); ret != 0 {
		return fmt.Errorf("failed to add vsock port listened on host, return: %v", ret)
	}

	return nil
}

func execCmdlineInVM(ctx context.Context, vmCtxID uint32) error {
	errChan := make(chan error, 1)
	go func() {
		logrus.Debugf("start enter vm with ctx id: %d", vmCtxID)
		if ret := C.krun_start_enter(C.uint32_t(vmCtxID)); ret != 0 {
			errChan <- fmt.Errorf("failed to start enter: %v", syscall.Errno(-ret))
		} else {
			errChan <- nil
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func setKernel(ctx context.Context, vmCtxID uint32, kernelImage, initramfs string, kernelCmdline ...string) error {
	// do some integrity check
	if ok, _ := filesystem.PathExists(initramfs); !ok {
		return fmt.Errorf("initramfs %q not exist", initramfs)
	}

	if ok, _ := filesystem.PathExists(kernelImage); !ok {
		return fmt.Errorf("kernel %q not exist", kernelImage)
	}

	if kernelCmdline != nil && len(kernelCmdline) == 0 {
		return fmt.Errorf("kernel cmdline is empty")
	}

	cKernel, func1 := GoString2CString(kernelImage)
	defer func1()

	cInitramfsPath, func2 := GoString2CString(initramfs)
	defer func2()

	var kcmdline strings.Builder
	for _, cmdline := range kernelCmdline {
		kcmdline.WriteString(cmdline)
		kcmdline.WriteString(" ")
	}

	cKernelCmdline, func3 := GoString2CString(kcmdline.String())
	defer func3()

	logrus.Debugf("set kernel: %q, initramfs: %q,  cmdline: %q", kernelImage, initramfs, kcmdline.String())

	if ret := C.krun_set_kernel(C.uint32_t(vmCtxID), cKernel, C.KRUN_KERNEL_FORMAT_RAW, cInitramfsPath, cKernelCmdline); ret != 0 {
		return fmt.Errorf("failed to set kernel/initramfs/cmdline, return %v", ret)
	}

	return nil
}
