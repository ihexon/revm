//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L  ../../out/3rd/darwin/lib  -lkrun.1 -lkrunfw.4
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"os"
	"path/filepath"
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
			C.free(unsafe.Pointer(str))
		}
	}
}

func GoString2CString(str string) (*C.char, func()) {
	cStr := C.CString(str)
	return cStr, func() {
		C.free(unsafe.Pointer(cStr))
	}
}

type AppleHVStubber struct {
	krunCtxID uint32
	vmc       *vmconfig.VMConfig
}

func (v *AppleHVStubber) IsSSHReady(ctx context.Context) bool {
	return true
}

func NewAppleHyperVisor(vmc *vmconfig.VMConfig) *AppleHVStubber {
	return &AppleHVStubber{
		vmc: vmc,
	}
}

func StartAPIServer(ctx context.Context) error {
	return nil
}

func (v *AppleHVStubber) Create(ctx context.Context) error {
	id := C.krun_create_ctx()
	if id < 0 {
		return fmt.Errorf("failed to create vm ctx id, return %v", id)
	}
	v.krunCtxID = uint32(id)

	if ret := C.krun_set_log_level(C.KRUN_LOG_LEVEL_INFO); ret != 0 {
		return fmt.Errorf("failed to set log level, return %v", ret)
	}

	if ret := C.krun_set_vm_config(C.uint32_t(v.krunCtxID), C.uint8_t(v.vmc.Cpus), C.uint32_t(v.vmc.MemoryInMB)); ret != 0 {
		return fmt.Errorf("failed to set vm config, return %v", ret)
	}

	if err := v.setRootFS(); err != nil {
		return err
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
	if err := v.addRawDisk(); err != nil {
		return err
	}

	if err := v.addVirtioFS(); err != nil {
		return err
	}
	if err := v.NestVirt(ctx); err != nil {
		return err
	}

	return v.writeCfgToRootfs(ctx)
}

func (v *AppleHVStubber) Start(ctx context.Context) error {
	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set rlimit: %v", err)
	}

	if err := v.setCommandLine(v.vmc.Cmdline.Workspace, v.vmc.Cmdline.Env); err != nil {
		return err
	}

	if err := system.Copy3rdFileTo(v.vmc.RootFS); err != nil {
		return fmt.Errorf("failed to copy 3rd files to rootfs: %w", err)
	}

	return execCmdlineInVM(ctx, v.krunCtxID)
}

func (v *AppleHVStubber) writeCfgToRootfs(ctx context.Context) error {
	return v.vmc.WriteToJsonFile(filepath.Join(v.vmc.RootFS, define.VMConfigFile))
}

func (v *AppleHVStubber) Stop(ctx context.Context) error {
	return stopVM(ctx, v.krunCtxID)
}

func (v *AppleHVStubber) setNetworkProvider() error {
	logrus.Infof("set vm network backend: %q", v.vmc.NetworkStackBackend)
	parse, err := url.Parse(v.vmc.NetworkStackBackend)
	if err != nil {
		return fmt.Errorf("failed to parse network stack backend: %w", err)
	}

	gvpSocket, fn1 := GoString2CString(parse.Path)
	defer fn1()

	if ret := C.krun_set_gvproxy_path(C.uint32_t(v.krunCtxID), gvpSocket); ret != 0 {
		return fmt.Errorf("failed to set gvproxy path, return %v", ret)
	}

	return nil
}

func (v *AppleHVStubber) GetVMConfigure() (*vmconfig.VMConfig, error) {
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

func (v *AppleHVStubber) setRLimited() error {
	limitStr, fn1 := GoStringList2CStringArray(
		[]string{fmt.Sprintf("%s=%s:%s", RLIMIT_NPROC, SoftLimit, HardLimit)},
	)
	defer fn1()

	if ret := C.krun_set_rlimits(C.uint32_t(v.krunCtxID), &limitStr[0]); ret != 0 {
		return fmt.Errorf("failed to set rlimits, return %v", ret)
	}
	return nil
}

func (v *AppleHVStubber) setCommandLine(dir string, env []string) error {
	workdir, fn1 := GoString2CString(dir)
	defer fn1()

	if ret := C.krun_set_workdir(C.uint32_t(v.krunCtxID), workdir); ret != 0 {
		return fmt.Errorf("failed to set workdir, return %v", ret)
	}

	targetBin, fn2 := GoString2CString(v.vmc.Cmdline.Bootstrap)
	defer fn2()

	targetBinArgs, fn3 := GoStringList2CStringArray([]string{})
	defer fn3()

	envPassIn, fn4 := GoStringList2CStringArray(env)
	defer fn4()

	if ret := C.krun_set_exec(C.uint32_t(v.krunCtxID), targetBin, &targetBinArgs[0], &envPassIn[0]); ret != 0 {
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

func (v *AppleHVStubber) setGPU() error {
	if err := C.krun_set_gpu_options(C.uint32_t(v.krunCtxID), C.uint32_t(VIRGLRENDERER_VENUS|VIRGLRENDERER_NO_VIRGL)); err != 0 {
		return fmt.Errorf("failed to set gpu options,return %v", err)
	}
	return nil
}

func (v *AppleHVStubber) AttachGuestConsole(ctx context.Context, rootfs string) {

}

func (v *AppleHVStubber) setRootFS() error {
	rootfs, fn1 := GoString2CString(v.vmc.RootFS)
	defer fn1()

	if ret := C.krun_set_root(C.uint32_t(v.krunCtxID), rootfs); ret != 0 {
		return fmt.Errorf("failed to set root, return %v", ret)
	}
	return nil
}

func (v *AppleHVStubber) addRawDisk() error {
	for _, disk := range v.vmc.DataDisk {
		if err := addRawDisk(v.krunCtxID, disk.Path); err != nil {
			return err
		}
	}

	return nil
}

func (v *AppleHVStubber) addVirtioFS() error {
	for _, mount := range v.vmc.Mounts {
		if err := addVirtioFS(v.krunCtxID, mount.Tag, mount.Source); err != nil {
			return fmt.Errorf("failed to add virtiofs: %w", err)
		}
	}

	return nil
}

func (v *AppleHVStubber) StartNetwork(ctx context.Context) error {
	return gvproxy.StartNetworking(ctx, v.vmc)
}

func (v *AppleHVStubber) NestVirt(ctx context.Context) error {
	if ret := C.krun_check_nested_virt(); ret == 0 {
		logrus.Info("current system not support nest virtualization, skip enable nested virtuallization")
		return nil
	} else if ret == 1 {
		logrus.Info("current system support nested virtuallization")
	} else {
		return fmt.Errorf("failed to check nested virtuallization support, return %v", ret)
	}

	if ret := C.krun_set_nested_virt(C.uint32_t(v.krunCtxID), true); ret != 0 {
		return fmt.Errorf("nested virtuallization support, but enable nested virtuallization failed")
	}

	logrus.Info("enable nested virtualization successful")

	return nil
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

	if ret := C.krun_add_virtiofs2(C.uint32_t(ctxID), cTag, cHostPath, C.uint64_t(1<<29)); ret != 0 {
		return fmt.Errorf("failed to add virtiofs, return: %v", ret)
	}

	return nil
}

func execCmdlineInVM(ctx context.Context, vmCtxID uint32) error {
	errChan := make(chan error, 1)
	go func() {
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
