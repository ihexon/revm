package libkrun

/*
#cgo CFLAGS: -I /Users/danhexon/Downloads/libkrun/1.11.2-pre/include
#cgo LDFLAGS: -L /Users/danhexon/Downloads/libkrun/1.11.2-pre/lib -lkrun -L /Users/danhexon/Downloads/libkrunfw/4.9.0/lib -l krunfw
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"fmt"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"syscall"
	"unsafe"
)

type loglevel int

const (
	OFF loglevel = iota
	ERROR
	WARN
	INFO
	DEBUG
	TRACE
)

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

func CreateVM(ctx context.Context, vmc *vmconfig.VMConfig, cmdline *vmconfig.Cmdline, loglevel loglevel) error {
	id := C.krun_create_ctx()
	if id < 0 {
		return fmt.Errorf("failed to create ctx: %v", syscall.Errno(-id))
	}
	ctxID := C.uint32_t(id)

	if ret := C.krun_set_log_level(C.uint32_t(loglevel)); ret != 0 {
		return fmt.Errorf("failed to set log level: %v", syscall.Errno(-ret))
	}

	if err := C.krun_set_vm_config(ctxID, C.uint8_t(vmc.Cpus), C.uint32_t(vmc.MemoryInMB)); err != 0 {
		return fmt.Errorf("failed to set vm config: %v", syscall.Errno(-err))
	}

	rootfs, defunct := GoString2CString(vmc.RootFS)
	defer defunct()
	// 显式进行类型转换
	if ret := C.krun_set_root(ctxID, (rootfs)); ret != 0 {
		return fmt.Errorf("failed to set root: %v", syscall.Errno(-ret))
	}

	virglFlags := VIRGLRENDERER_VENUS | VIRGLRENDERER_NO_VIRGL

	virgl_flags := C.uint32_t(virglFlags)
	if err := C.krun_set_gpu_options(ctxID, virgl_flags); err != 0 {
		return fmt.Errorf("Failed to set gpu options: %v\n", syscall.Errno(-err))
	}

	CLimitStr, defunct2 := GoStringList2CStringArray([]string{"6=4096:8192"})
	defer defunct2()
	if err := C.krun_set_rlimits(ctxID, &CLimitStr[0]); err != 0 {
		return fmt.Errorf("Failed to set rlimits: %v\n", syscall.Errno(-err))
	}

	workdir, defunct3 := GoString2CString("/")
	defer defunct3()
	if err := C.krun_set_workdir(ctxID, workdir); err != 0 {
		return fmt.Errorf("Failed to set workdir: %v\n", syscall.Errno(-err))
	}

	targetBin, defunct4 := GoString2CString(cmdline.TargetBin)
	defer defunct4()

	targetBinArgs, defunct5 := GoStringList2CStringArray(cmdline.TargetBinArgs)
	defer defunct5()

	envPassIn, defunct6 := GoStringList2CStringArray([]string{"TEST=works"})
	defer defunct6()

	if ret := C.krun_set_exec(ctxID, targetBin, &targetBinArgs[0], &envPassIn[0]); ret != 0 {
		return fmt.Errorf("Failed to set exec: %v\n", syscall.Errno(-ret))
	}

	parse, err := url.Parse(vmc.NetworkStackBackend)
	if err != nil {
		return fmt.Errorf("Failed to parse url: %v\n", err)
	}
	vfkitSocketPath := parse.Path

	gvproxyVfkitSocket, defunct7 := GoString2CString(vfkitSocketPath)
	defer defunct7()
	if ret := C.krun_set_gvproxy_path(ctxID, gvproxyVfkitSocket); ret != 0 {
		return fmt.Errorf("Failed to set gvproxy path: %v\n", syscall.Errno(-ret))
	}

	blockID, _ := GoString2CString("1")
	disk, _ := GoString2CString("/Users/danhexon/disk")
	if ret := C.krun_add_disk(ctxID, blockID, disk, false); ret != 0 {
		return fmt.Errorf("Failed to add disk: %v\n", syscall.Errno(-ret))
	}

	if ret := C.krun_start_enter(ctxID); ret != 0 {
		return fmt.Errorf("Failed to start enter: %v\n", syscall.Errno(-ret))
	}

	return nil
}
