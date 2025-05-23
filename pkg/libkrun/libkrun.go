package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L ../../lib  -lkrun -lkrunfw
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"os"
	"syscall"
	"unsafe"
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

func StartVM(ctx context.Context, vmc vmconfig.VMConfig, cmdline vmconfig.Cmdline) error {
	vm, err := NewVM(vmc)
	if err != nil {
		return fmt.Errorf("failed to create VM: %v", err)
	}

	vm, err = vm.SetRootFS()
	if err != nil {
		return fmt.Errorf("set rootfs err: %v", err)
	}

	vm, err = vm.SetGPU()
	if err != nil {
		return fmt.Errorf("set gpu err: %v", err)
	}

	vm, err = vm.SetRLimited()
	if err != nil {
		return fmt.Errorf("set rlimited err: %v", err)
	}

	vm, err = vm.SetCommandLine(cmdline.Workspace, cmdline.Env, cmdline.TargetBin, cmdline.TargetBinArgs...)
	if err != nil {
		return fmt.Errorf("set cmdline err: %v", err)
	}

	vm, err = vm.SetNetworkProvider()
	if err != nil {
		return fmt.Errorf("set network provider err: %v", err)
	}

	vm, err = vm.AddDisk()
	if err != nil {
		return fmt.Errorf("set disk err: %v", err)
	}

	return vm.StartEnter()
}

type VMInfo struct {
	vmc     vmconfig.VMConfig
	Cmdline vmconfig.Cmdline
}

func NewVM(vmc vmconfig.VMConfig) (*VMInfo, error) {
	// int32_t krun_create_ctx();
	id := C.krun_create_ctx()
	if id < 0 {
		return nil, fmt.Errorf("failed to create ctx: %v", syscall.Errno(-id))
	}

	vmInfo := &VMInfo{
		vmc: vmc,
	}

	if ret := C.krun_set_log_level(C.uint32_t(define.LogLevelStr2Type(vmInfo.vmc.LogLevel))); ret != 0 {
		return nil, fmt.Errorf("failed to set log level: %v", syscall.Errno(-ret))
	}

	if err := C.krun_set_vm_config(C.uint32_t(vmInfo.vmc.CtxID), C.uint8_t(vmc.Cpus), C.uint32_t(vmc.MemoryInMB)); err != 0 {
		return nil, fmt.Errorf("failed to set vm config: %v", syscall.Errno(-err))
	}

	return vmInfo, nil
}

func (v *VMInfo) SetNetworkProvider() (*VMInfo, error) {
	parse, err := url.Parse(v.vmc.NetworkStackBackend)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %v", err)
	}

	gvpSocket, defunct := GoString2CString(parse.Path)
	defer defunct()

	if ret := C.krun_set_gvproxy_path(C.uint32_t(v.vmc.CtxID), gvpSocket); ret != 0 {
		return nil, fmt.Errorf("failed to set gvproxy path: %v", syscall.Errno(-ret))
	}

	return v, nil
}

const (
	RLIMIT_NPROC = "6"
	SoftLimit    = "4096"
	HardLimit    = "8192"
)

func (v *VMInfo) SetRLimited() (*VMInfo, error) {
	limitStr, defunct := GoStringList2CStringArray(
		[]string{fmt.Sprintf("%s=%s:%s", RLIMIT_NPROC, SoftLimit, HardLimit)},
	)
	defer defunct()

	if err := C.krun_set_rlimits(C.uint32_t(v.vmc.CtxID), &limitStr[0]); err != 0 {
		return nil, fmt.Errorf("failed to set rlimits: %v", syscall.Errno(-err))
	}
	return v, nil
}

func (v *VMInfo) SetCommandLine(dir string, env []string, bin string, args ...string) (*VMInfo, error) {
	workdir, defunct := GoString2CString(dir)
	defer defunct()

	if err := C.krun_set_workdir(C.uint32_t(v.vmc.CtxID), workdir); err != 0 {
		return nil, fmt.Errorf("failed to set workdir: %v", syscall.Errno(-err))
	}

	if bin == "" {
		return nil, fmt.Errorf("no cmdline provided")
	}

	v.Cmdline.TargetBin = bin
	v.Cmdline.TargetBinArgs = args

	targetBin, defunct2 := GoString2CString(v.Cmdline.TargetBin)
	defer defunct2()

	targetBinArgs, defunct3 := GoStringList2CStringArray(v.Cmdline.TargetBinArgs)
	defer defunct3()

	envPassIn, defunct4 := GoStringList2CStringArray(env)
	defer defunct4()

	if ret := C.krun_set_exec(C.uint32_t(v.vmc.CtxID), targetBin, &targetBinArgs[0], &envPassIn[0]); ret != 0 {
		return nil, fmt.Errorf("failed to set exec: %v", syscall.Errno(-ret))
	}

	return v, nil
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

func (v *VMInfo) SetGPU() (*VMInfo, error) {
	if err := C.krun_set_gpu_options(C.uint32_t(v.vmc.CtxID), C.uint32_t(VIRGLRENDERER_VENUS|VIRGLRENDERER_NO_VIRGL)); err != 0 {
		return nil, fmt.Errorf("failed to set gpu options: %v", syscall.Errno(-err))
	}
	return v, nil
}

func (v *VMInfo) SetRootFS() (*VMInfo, error) {
	rootfs, defunct := GoString2CString(v.vmc.RootFS)
	defer defunct()

	if ret := C.krun_set_root(C.uint32_t(v.vmc.CtxID), rootfs); ret != 0 {
		return nil, fmt.Errorf("failed to set root: %v", syscall.Errno(-ret))
	}
	return v, nil
}

func (v *VMInfo) AddDisk() (*VMInfo, error) {
	for _, disk := range v.vmc.DataDisk {
		if err := addDisk(v.vmc.CtxID, disk); err != nil {
			return v, err
		}
	}

	return v, nil
}

func (v *VMInfo) StartEnter() error {
	if ret := C.krun_start_enter(C.uint32_t(v.vmc.CtxID)); ret != 0 {
		return fmt.Errorf("failed to start enter: %v", syscall.Errno(-ret))
	}
	return nil
}

func addDisk(ctxID uint32, disk string) error {
	_, err := os.Stat(disk)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%q disk not exist", disk)
	}

	blockID, freeFunc := GoString2CString(uuid.New().String())
	defer freeFunc()

	extDisk, freeFunc2 := GoString2CString(disk)
	defer freeFunc2()

	if ret := C.krun_add_disk(C.uint32_t(ctxID), blockID, extDisk, false); ret != 0 {
		return fmt.Errorf("failed to add disk: %v", syscall.Errno(-ret))
	}

	return nil
}
