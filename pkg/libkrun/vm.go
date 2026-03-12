//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I /tmp/.deps/libkrun/include
#cgo LDFLAGS: -L /tmp/.deps/libkrun/lib/ -L/tmp/.deps/libkrunfw/lib -lkrun -lkrunfw
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"runtime"
	"runtime/debug"
	"unsafe"

	"github.com/sirupsen/logrus"
)

// VM wraps libkrun context and manages VM lifecycle.
type VM struct {
	cfg   *define.Machine
	ctxID uint32

	// Keep file references to prevent GC
	files struct {
		stdin, stdout, stderr *os.File
		consolePty            [2]*os.File // master, slave
		guestLog              *os.File
		signalPipe            *os.File // write end
	}
}

// New creates a new VM instance.
func New(cfg *define.Machine) *VM {
	return &VM{cfg: cfg}
}

// Create initializes the VM configuration.
func (v *VM) Create(ctx context.Context) error {
	if err := v.init(); err != nil {
		return err
	}

	v.setResources()
	v.setRootFS()

	if err := v.setupConsole(); err != nil {
		return err
	}
	if err := v.setupVSock(); err != nil {
		return err
	}
	if err := v.setupNetwork(); err != nil {
		return err
	}
	if err := v.setupStorage(); err != nil {
		return err
	}

	v.setupGPU()
	v.setupNestedVirt()
	v.setGuestAgent()

	return nil
}

// Start launches the VM and blocks until it exits.
func (v *VM) Start(ctx context.Context) error {
	ret := C.krun_start_enter(C.uint32_t(v.ctxID))

	// Cleanup
	if v.files.signalPipe != nil {
		_ = v.files.signalPipe.Close()
	}

	if ret != 0 {
		return fmt.Errorf("VM failed: %w", errCode(ret))
	}
	return nil
}

// SendSignal writes a signal message to the VM's signal pipe.
func (v *VM) SendSignal(name string) {
	if v.files.signalPipe == nil {
		return
	}
	msg := struct{ SignalName string }{SignalName: name}
	if b, err := json.Marshal(msg); err == nil {
		_, _ = v.files.signalPipe.Write(b)
		_, _ = v.files.signalPipe.Write([]byte("\n"))
	}
}

// init creates libkrun context and initializes logging.
func (v *VM) init() error {
	level := logLevel(os.Getenv("LIBKRUN_DEBUG"))
	if ret := C.krun_init_log(C.KRUN_LOG_TARGET_DEFAULT, level, C.KRUN_LOG_STYLE_AUTO, C.KRUN_LOG_OPTION_NO_ENV); ret != 0 {
		return errCode(ret)
	}

	ctxID := C.krun_create_ctx()
	if ctxID < 0 {
		return errCode(ctxID)
	}
	v.ctxID = uint32(ctxID)

	return nil
}

// setResources configures CPU, memory, and limits.
func (v *VM) setResources() {
	must(C.krun_set_vm_config(
		C.uint32_t(v.ctxID),
		C.uint8_t(v.cfg.Cpus),
		C.uint32_t(v.cfg.MemoryInMB),
	))

	rlimits := cstrings("6=4096:8192") // RLIMIT_NPROC
	defer rlimits.free()
	must(C.krun_set_rlimits(C.uint32_t(v.ctxID), rlimits.ptr()))
}

// setRootFS sets the root filesystem path.
func (v *VM) setRootFS() {
	rootfs := cstr(v.cfg.RootFS)
	defer free(rootfs)
	must(C.krun_set_root(C.uint32_t(v.ctxID), rootfs))
}

// setGuestAgent configures the guest agent executable.
func (v *VM) setGuestAgent() {
	workdir := cstr(v.cfg.GuestAgentCfg.Workdir)
	defer free(workdir)
	must(C.krun_set_workdir(C.uint32_t(v.ctxID), workdir))

	exec := cstr(define.GuestAgentPathInGuest)
	defer free(exec)

	args := cstrings(v.cfg.GuestAgentCfg.Args...)
	defer args.free()

	envs := cstrings(v.cfg.GuestAgentCfg.Env...)
	defer envs.free()

	must(C.krun_set_exec(C.uint32_t(v.ctxID), exec, args.ptr(), envs.ptr()))
}

// setupGPU enables GPU passthrough on macOS.
func (v *VM) setupGPU() {
	if runtime.GOOS != "darwin" {
		return
	}
	const gpuFlags = (1 << 6) | (1 << 7) // Venus + NoVirgl
	_ = C.krun_set_gpu_options(C.uint32_t(v.ctxID), C.uint32_t(gpuFlags))
}

// setupNestedVirt enables nested virtualization if supported.
func (v *VM) setupNestedVirt() {
	if C.krun_check_nested_virt() == 1 {
		_ = C.krun_set_nested_virt(C.uint32_t(v.ctxID), true)
	}
}

// Helper functions

func cstr(s string) *C.char {
	return C.CString(s)
}

func free(p *C.char) {
	C.free(unsafe.Pointer(p))
}

type cstringArray struct {
	ptrs []*C.char
}

func cstrings(strs ...string) *cstringArray {
	ptrs := make([]*C.char, len(strs)+1)
	for i, s := range strs {
		ptrs[i] = C.CString(s)
	}
	return &cstringArray{ptrs: ptrs}
}

func (a *cstringArray) ptr() **C.char {
	if len(a.ptrs) == 0 {
		return nil
	}
	return &a.ptrs[0]
}

func (a *cstringArray) free() {
	for i, p := range a.ptrs {
		if p != nil {
			C.free(unsafe.Pointer(p))
			a.ptrs[i] = nil
		}
	}
}

func must(ret C.int32_t) {
	if ret != 0 {
		err := errCode(ret)
		logrus.Errorf("libkrun fatal error: %v", err)
		// Log stack trace for debugging
		logrus.Errorf("stack trace: %s", debug.Stack())
		panic(err)
	}
}

func errCode(code C.int32_t) error {
	return fmt.Errorf("libkrun error: %d", code)
}

func logLevel(env string) C.uint32_t {
	switch env {
	case "trace":
		return C.KRUN_LOG_LEVEL_TRACE
	case "debug", "1":
		return C.KRUN_LOG_LEVEL_DEBUG
	case "info":
		return C.KRUN_LOG_LEVEL_INFO
	case "warn":
		return C.KRUN_LOG_LEVEL_WARN
	default:
		return C.KRUN_LOG_LEVEL_ERROR
	}
}
