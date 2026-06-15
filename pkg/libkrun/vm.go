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
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/static_resources"
	"os"
	"runtime"
	"sync"
	"unsafe"
)

const (
	rootFSTag            = "/dev/root"
	defaultInitPath      = "init.krun"
	guestHiddenBinDir    = ".bin"
	guestAgentPath       = ".bin/guest-agent"
	overlayFileMode      = 0755
	overlayDirectoryMode = 0755
)

// Libkrun wraps Libkrun context and manages Libkrun lifecycle.
type Libkrun struct {
	cfg   *define.MachineSpec
	ctxID uint32

	files          libkrunFiles
	guestAgentData unsafe.Pointer

	ctxCreated bool
	closeOnce  sync.Once
	closeErr   error
}

// New creates a new Libkrun instance.
func New(cfg *define.MachineSpec) *Libkrun {
	return &Libkrun{cfg: cfg}
}

// Create initializes the Libkrun configuration.
func (v *Libkrun) Create(ctx context.Context) (retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, v.Close())
		}
	}()

	if err := v.init(); err != nil {
		return err
	}

	if err := v.setResources(); err != nil {
		return err
	}
	if err := v.disableImplicitInit(); err != nil {
		return err
	}
	if err := v.setRootFS(); err != nil {
		return err
	}
	if err := v.setupRootOverlay(); err != nil {
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
	v.files.startConsoleIO()
	ret := C.krun_start_enter(C.uint32_t(v.ctxID))
	if ret != 0 {
		return fmt.Errorf("Libkrun failed: %w", errCode(ret))
	}

	return nil
}

// Close releases the libkrun configuration context and the host-side files that
// were kept alive for raw fd ownership. It is idempotent.
func (v *Libkrun) Close() error {
	v.closeOnce.Do(func() {
		v.closeErr = v.close()
	})
	return v.closeErr
}

func (v *Libkrun) close() error {
	var err error
	if v.ctxCreated {
		if ret := C.krun_free_ctx(C.uint32_t(v.ctxID)); ret != 0 {
			err = errors.Join(err, errCode(ret))
		}
		v.ctxCreated = false
		v.ctxID = 0
	}
	v.files.close()
	v.freeGuestAgentData()
	return err
}

// SendSignal writes a signal message to the Libkrun's signal pipe.
func (v *Libkrun) SendSignal(ctx context.Context, name define.GuestSignalName) error {
	if v.files.signalPipe.write == nil {
		return nil
	}

	msg := define.GuestSignal{SignalName: name}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return writeSignalMessage(ctx, v.files.signalPipe.write.file, append(b, '\n'))
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
	v.ctxCreated = true

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

func (v *Libkrun) disableImplicitInit() error {
	if ret := C.krun_disable_implicit_init(C.uint32_t(v.ctxID)); ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (v *Libkrun) setupRootOverlay() error {
	if err := v.addDefaultInitOverlay(); err != nil {
		return err
	}
	if err := v.addOverlayDir(guestHiddenBinDir, overlayDirectoryMode); err != nil {
		return err
	}
	return v.addGuestAgentOverlay()
}

func (v *Libkrun) addDefaultInitOverlay() error {
	var data *C.uint8_t
	var dataLen C.size_t
	if ret := C.krun_get_default_init(&data, &dataLen); ret != 0 {
		return errCode(ret)
	}
	return v.addOverlayFile(defaultInitPath, data, dataLen, overlayFileMode, true)
}

func (v *Libkrun) addGuestAgentOverlay() error {
	guestAgent, err := static_resources.GuestAgent()
	if err != nil {
		return err
	}

	v.freeGuestAgentData()
	v.guestAgentData = C.CBytes(guestAgent)
	if v.guestAgentData == nil {
		return fmt.Errorf("failed to allocate guest-agent overlay")
	}

	data := (*C.uint8_t)(v.guestAgentData)
	dataLen := C.size_t(len(guestAgent))
	if err := v.addOverlayFile(guestAgentPath, data, dataLen, overlayFileMode, false); err != nil {
		v.freeGuestAgentData()
		return err
	}
	return nil
}

func (v *Libkrun) addOverlayFile(path string, data *C.uint8_t, dataLen C.size_t, mode C.uint32_t, oneShot bool) error {
	tagC := cstr(rootFSTag)
	defer free(tagC)

	pathC := cstr(path)
	defer free(pathC)

	ret := C.krun_fs_add_overlay_file(
		C.uint32_t(v.ctxID),
		tagC,
		pathC,
		data,
		dataLen,
		mode,
		C.bool(oneShot),
	)
	if ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (v *Libkrun) addOverlayDir(path string, mode C.uint32_t) error {
	tagC := cstr(rootFSTag)
	defer free(tagC)

	pathC := cstr(path)
	defer free(pathC)

	if ret := C.krun_fs_add_overlay_dir(C.uint32_t(v.ctxID), tagC, pathC, mode); ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (v *Libkrun) freeGuestAgentData() {
	if v.guestAgentData != nil {
		C.free(v.guestAgentData)
		v.guestAgentData = nil
	}
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
