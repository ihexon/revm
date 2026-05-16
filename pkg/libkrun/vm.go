//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
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
	"unsafe"
)

// Libkrun wraps Libkrun context and manages Libkrun lifecycle.
type Libkrun struct {
	cfg   *define.MachineSpec
	ctxID uint32

	// Keep file references to prevent GC
	files struct {
		stdin, stdout, stderr *os.File
		consolePty            [2]*os.File // master, slave
		guestLog              *os.File
		signalPipeR           *os.File // read end kept alive for Libkrun
		signalPipeW           *os.File // write end
	}
}

// New creates a new Libkrun instance.
func New(cfg *define.MachineSpec) *Libkrun {
	return &Libkrun{cfg: cfg}
}

// Create initializes the Libkrun configuration.
func (v *Libkrun) Create(ctx context.Context) error {
	if err := v.init(); err != nil {
		return err
	}

	if err := v.setResources(); err != nil {
		return err
	}
	if err := v.setRootFS(); err != nil {
		return err
	}

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
	if err := v.setGuestAgent(); err != nil {
		return err
	}

	return nil
}

// Start launches Libkrun and blocks until it exits. vmWaitAbortCtx names the caller's
// wait-abort context; graceful guest shutdown is requested outside this method.
func (v *Libkrun) Start(vmWaitAbortCtx context.Context) error {
	// krun_start_enter 后的代码在 https://github.com/containers/libkrun/issues/561 得到修复前，
	// 永远没有机会执行，因为 Libkrun 会使用 exit 退出程序
	//
	// 但我仍然做象征意义上的清理工作，因为这样让人感到愉悦
	ret := C.krun_start_enter(C.uint32_t(v.ctxID))

	// 让人愉悦但永远没机会执行的代码
	if v.files.signalPipeR != nil {
		_ = v.files.signalPipeR.Close()
	}
	if v.files.signalPipeW != nil {
		_ = v.files.signalPipeW.Close()
	}
	if v.files.guestLog != nil {
		_ = v.files.guestLog.Close()
	}
	if v.files.consolePty[0] != nil {
		_ = v.files.consolePty[0].Close()
	}
	if v.files.consolePty[1] != nil {
		_ = v.files.consolePty[1].Close()
	}
	if v.files.stdin != nil {
		_ = v.files.stdin.Close()
	}
	if v.files.stdout != nil {
		_ = v.files.stdout.Close()
	}
	if v.files.stderr != nil {
		_ = v.files.stderr.Close()
	}

	if ret != 0 {
		return fmt.Errorf("Libkrun failed: %w", errCode(ret))
	}

	return nil
}

// SendSignal writes a signal message to the Libkrun's signal pipe.
func (v *Libkrun) SendSignal(ctx context.Context, name define.GuestSignalName) error {
	if v.files.signalPipeW == nil {
		return nil
	}

	msg := define.GuestSignal{SignalName: name}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return writeSignalMessage(ctx, v.files.signalPipeW, append(b, '\n'))
}

func writeSignalMessage(ctx context.Context, f *os.File, msg []byte) error {
	errCh := make(chan error, 1)
	go func() {
		_, err := f.Write(msg)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// init creates Libkrun context and initializes logging.
func (v *Libkrun) init() error {
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
func (v *Libkrun) setResources() error {
	if ret := C.krun_set_vm_config(
		C.uint32_t(v.ctxID),
		C.uint8_t(v.cfg.Cpus),
		C.uint32_t(v.cfg.MemoryInMB),
	); ret != 0 {
		return errCode(ret)
	}

	rlimits := cstrings("6=4096:8192") // RLIMIT_NPROC
	defer rlimits.free()
	if ret := C.krun_set_rlimits(C.uint32_t(v.ctxID), rlimits.ptr()); ret != 0 {
		return errCode(ret)
	}
	return nil
}

// setRootFS sets the root filesystem path.
func (v *Libkrun) setRootFS() error {
	rootfs := cstr(v.cfg.RootFS)
	defer free(rootfs)
	if ret := C.krun_set_root(C.uint32_t(v.ctxID), rootfs); ret != 0 {
		return errCode(ret)
	}
	return nil
}

// setGuestAgent configures the guest agent executable.
func (v *Libkrun) setGuestAgent() error {
	workdir := cstr(v.cfg.GuestAgentCfg.Workdir)
	defer free(workdir)
	if ret := C.krun_set_workdir(C.uint32_t(v.ctxID), workdir); ret != 0 {
		return errCode(ret)
	}

	exec := cstr(define.GuestAgentPathInGuest)
	defer free(exec)

	args := cstrings(v.cfg.GuestAgentCfg.Args...)
	defer args.free()

	envs := cstrings(v.cfg.GuestAgentCfg.Env...)
	defer envs.free()

	if ret := C.krun_set_exec(C.uint32_t(v.ctxID), exec, args.ptr(), envs.ptr()); ret != 0 {
		return errCode(ret)
	}
	return nil
}

// setupGPU enables GPU passthrough on macOS.
func (v *Libkrun) setupGPU() {
	if runtime.GOOS != "darwin" {
		return
	}
	const gpuFlags = (1 << 6) | (1 << 7) // Venus + NoVirgl
	_ = C.krun_set_gpu_options(C.uint32_t(v.ctxID), C.uint32_t(gpuFlags))
}

// setupNestedVirt enables nested virtualization if supported.
func (v *Libkrun) setupNestedVirt() {
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

func errCode(code C.int32_t) error {
	return fmt.Errorf("Libkrun error: %d", code)
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
