//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
#include <libkrun_init.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

const (
	rootFSTag            = "/dev/root"
	guestHiddenBinDir    = ".bin"
	guestAgentPath       = ".bin/guest-agent"
	overlayFileMode      = 0755
	overlayDirectoryMode = 0755
	virtiofsMemWindow    = 512 << 20
	vsockCID             = 3
)

// Libkrun owns one libkrun 2.x builder graph. The graph is kept alive until
// krun_vmm_run consumes the VMM; initConfig is kept until the VMM exits because
// libkrun_init borrows the serialized configuration data.
type Libkrun struct {
	cfg *define.MachineSpec

	manager    C.KrunMmioDeviceManager
	payload    C.KrunPayload
	initConfig C.KrunInitConfig
	vmmBuilder C.KrunVmmBuilder
	vmm        C.KrunVmm
	vmmHandle  C.KrunVmmHandle

	console C.KrunConsoleDevice
	files   libkrunFiles

	guestAgentData unsafe.Pointer
	closeOnce      sync.Once
	closeErr       error
}

// New creates a new libkrun instance.
func New(cfg *define.MachineSpec) *Libkrun { return &Libkrun{cfg: cfg} }

// Create constructs a libkrun 2.x VMM. Handles are transferred to the manager
// or builder as soon as they are added, which makes cleanup deterministic.
func (v *Libkrun) Create(ctx context.Context) (retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, v.Close())
		}
	}()
	if v.cfg == nil {
		return errors.New("libkrun: nil machine specification")
	}
	if err := v.init(); err != nil {
		return err
	}
	if err := v.setResources(); err != nil {
		return err
	}
	if err := v.loadPayload(); err != nil {
		return err
	}
	if err := v.setupInitConfig(); err != nil {
		return err
	}
	if err := v.setupDevices(); err != nil {
		return err
	}
	return v.buildVMM()
}

// Start enters the VMM. The upstream API consumes the VMM handle and reports
// failures through libkrun's logging/error path, so this returns when the guest
// has stopped.
func (v *Libkrun) Start(_ context.Context) error {
	if v.vmm == nil {
		return errors.New("libkrun: VMM has not been created")
	}
	v.files.startConsoleIO()
	C.krun_vmm_run(v.vmm)
	v.vmm = nil
	return nil
}

// Shutdown asks libkrun to perform its native guest shutdown. It is safe to
// call while krun_vmm_run is blocked; the handle is designed for that purpose.
func (v *Libkrun) Shutdown(_ context.Context) error {
	if v.vmmHandle == nil {
		return errors.New("libkrun: VMM handle is unavailable")
	}
	var errOut C.KrunError
	return checkKrunResult("shutdown VMM", C.krun_vmm_handle_shutdown(v.vmmHandle, &errOut), errOut)
}

func (v *Libkrun) Pause(_ context.Context) error {
	if v.vmmHandle == nil {
		return errors.New("libkrun: VMM handle is unavailable")
	}
	var errOut C.KrunError
	return checkKrunResult("pause VMM", C.krun_vmm_handle_pause(v.vmmHandle, &errOut), errOut)
}

func (v *Libkrun) Resume(_ context.Context) error {
	if v.vmmHandle == nil {
		return errors.New("libkrun: VMM handle is unavailable")
	}
	var errOut C.KrunError
	return checkKrunResult("resume VMM", C.krun_vmm_handle_resume(v.vmmHandle, &errOut), errOut)
}

// Close releases handles and host file descriptors. It is idempotent and also
// cleans up partially built graphs after a failed Create call.
func (v *Libkrun) Close() error {
	v.closeOnce.Do(func() { v.closeErr = v.close() })
	return v.closeErr
}

func (v *Libkrun) close() error {
	if v.vmmHandle != nil {
		C.krun_vmm_handle_destroy(v.vmmHandle)
		v.vmmHandle = nil
	}
	if v.vmm != nil {
		C.krun_vmm_destroy(v.vmm)
		v.vmm = nil
	}
	if v.vmmBuilder != nil {
		C.krun_vmm_builder_destroy(v.vmmBuilder)
		v.vmmBuilder = nil
	}
	if v.manager != nil {
		C.krun_mmio_device_manager_destroy(v.manager)
		v.manager = nil
	}
	if v.payload != nil {
		C.krun_payload_destroy(v.payload)
		v.payload = nil
	}
	if v.initConfig != nil {
		C.krun_init_config_destroy(v.initConfig)
		v.initConfig = nil
	}
	if v.console != nil {
		C.krun_console_device_destroy(v.console)
		v.console = nil
	}
	v.files.close()
	v.freeGuestAgentData()
	return nil
}

// SendSignal writes a signal message to the guest-signal console port.
func (v *Libkrun) SendSignal(ctx context.Context, name define.GuestSignalName) error {
	if v.files.signalPipe.write == nil {
		return nil
	}
	b, err := json.Marshal(define.GuestSignal{SignalName: name})
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

func (v *Libkrun) init() error {
	var errOut C.KrunError
	// The optional BorrowedFd parameter uses -1 for the default logger target.
	result := C.krun_init_log(-1, logLevel(os.Getenv("LIBKRUN_DEBUG")), C.KRUN_LOG_STYLE_AUTO, C.KRUN_LOG_OPTIONS_NO_ENV, &errOut)
	if err := checkKrunResult("initialize logging", result, errOut); err != nil {
		return err
	}
	v.manager = C.krun_mmio_device_manager_new()
	if v.manager == nil {
		return errors.New("libkrun: failed to allocate device manager")
	}
	return nil
}

func (v *Libkrun) setResources() error {
	v.vmmBuilder = C.krun_vmm_builder_new()
	if v.vmmBuilder == nil {
		return errors.New("libkrun: failed to allocate VMM builder")
	}
	var errOut C.KrunError
	if err := checkKrunResult("set vCPU count", C.krun_vmm_builder_vcpus(&v.vmmBuilder, C.uint8_t(v.cfg.Cpus), &errOut), errOut); err != nil {
		return err
	}
	errOut = nil
	return checkKrunResult("set memory size", C.krun_vmm_builder_ram_mib(&v.vmmBuilder, C.uint32_t(v.cfg.MemoryInMB), &errOut), errOut)
}

func (v *Libkrun) loadPayload() error {
	var errOut C.KrunError
	v.payload = C.krun_payload_load_krunfw(&errOut)
	return checkKrunHandle("load libkrunfw payload", unsafe.Pointer(v.payload), errOut)
}

func (v *Libkrun) setupInitConfig() error {
	builder := C.krun_init_config_builder()
	if builder == nil {
		return errors.New("libkrun: failed to allocate init config builder")
	}
	defer func() {
		if builder != nil {
			C.krun_init_builder_destroy(builder)
		}
	}()

	initBuilderArg(&builder, define.GuestAgentPathInGuest)
	for _, arg := range v.cfg.GuestAgentCfg.Args {
		initBuilderArg(&builder, arg)
	}
	workdir := v.cfg.GuestAgentCfg.Workdir
	if workdir == "" {
		workdir = "/"
	}
	workdirStr := newKrunStr(workdir)
	C.krun_init_builder_workdir(&builder, workdirStr.value)
	workdirStr.free()
	rlimit := newKrunStr("6=4096:8192") // RLIMIT_NPROC
	C.krun_init_builder_rlimit(&builder, rlimit.value)
	rlimit.free()
	for _, env := range v.cfg.GuestAgentCfg.Env {
		envStr := newKrunStr(env)
		C.krun_init_builder_env_var(&builder, envStr.value)
		envStr.free()
	}
	v.initConfig = C.krun_init_builder_build(&builder)
	builder = nil
	if v.initConfig == nil {
		return errors.New("libkrun: failed to build init config")
	}
	return nil
}

func (v *Libkrun) setupDevices() error {
	if err := v.setupRootFS(); err != nil {
		return err
	}
	if err := v.setupConsole(); err != nil {
		return err
	}
	if err := v.setupVSock(); err != nil {
		return err
	}
	if err := v.setupRNG(); err != nil {
		return err
	}
	if err := v.setupBalloon(); err != nil {
		return err
	}
	if err := v.setupNetwork(); err != nil {
		return err
	}
	return v.setupStorage()
}

func (v *Libkrun) setupRNG() error {
	var errOut C.KrunError
	device := C.krun_rng_device_new(&errOut)
	if err := checkKrunHandle("create RNG device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}

func (v *Libkrun) setupBalloon() error {
	var errOut C.KrunError
	device := C.krun_balloon_device_new(&errOut)
	if err := checkKrunHandle("create virtio-balloon device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}

func (v *Libkrun) setupRootFS() error {
	rootPath, err := filepath.Abs(v.cfg.RootFS)
	if err != nil {
		return fmt.Errorf("resolve rootfs: %w", err)
	}
	if err := filesystem.EnsureVirtioFSRootOwnership(rootPath); err != nil {
		return fmt.Errorf("normalize root virtiofs ownership: %w", err)
	}
	root := C.KrunFsDevice(nil)
	var errOut C.KrunError
	tag := newKrunStr(rootFSTag)
	path := newKrunStr(rootPath)
	root = C.krun_fs_device_new(tag.value, path.value, &errOut)
	tag.free()
	path.free()
	if err := checkKrunHandle("create root filesystem", unsafe.Pointer(root), errOut); err != nil {
		return err
	}
	defer func() {
		if root != nil {
			C.krun_fs_device_destroy(root)
		}
	}()
	C.krun_fs_device_set_dax_window_size(root, C.uint64_t(virtiofsMemWindow))

	overlay := C.krun_fs_overlay_new()
	if overlay == nil {
		return errors.New("libkrun: failed to allocate root filesystem overlay")
	}
	defer func() {
		if overlay != nil {
			C.krun_fs_overlay_destroy(overlay)
		}
	}()
	if err := addOverlayDir(overlay, guestHiddenBinDir, overlayDirectoryMode); err != nil {
		return err
	}
	if err := v.addGuestAgentOverlay(overlay); err != nil {
		return err
	}
	var initErr C.KrunInitError
	result := C.krun_init_config_apply(v.initConfig, overlay, v.payload, &initErr)
	if err := checkInitResult("apply init config", result, initErr); err != nil {
		return err
	}
	C.krun_fs_device_set_overlay(root, overlay)
	overlay = nil
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(root)))
	root = nil
	return nil
}

func (v *Libkrun) addGuestAgentOverlay(overlay C.KrunFsOverlay) error {
	guestAgent, err := static_resources.GuestAgent()
	if err != nil {
		return err
	}
	v.freeGuestAgentData()
	v.guestAgentData = C.CBytes(guestAgent)
	if v.guestAgentData == nil {
		return errors.New("libkrun: failed to allocate guest-agent overlay")
	}
	data := C.KrunBytes{data: (*C.uint8_t)(v.guestAgentData), len: C.size_t(len(guestAgent))}
	return addOverlayFile(overlay, guestAgentPath, data, overlayFileMode, false)
}

func addOverlayDir(overlay C.KrunFsOverlay, path string, mode C.uint32_t) error {
	p := newKrunStr(path)
	defer p.free()
	var errOut C.KrunError
	return checkKrunResult("add overlay directory", C.krun_fs_overlay_add_dir(overlay, p.value, mode, &errOut), errOut)
}

func addOverlayFile(overlay C.KrunFsOverlay, path string, data C.KrunBytes, mode C.uint32_t, oneShot bool) error {
	p := newKrunStr(path)
	defer p.free()
	var errOut C.KrunError
	return checkKrunResult("add overlay file", C.krun_fs_overlay_add_file(overlay, p.value, data, mode, C.bool(oneShot), &errOut), errOut)
}

func (v *Libkrun) buildVMM() error {
	C.krun_vmm_builder_payload(&v.vmmBuilder, v.payload)
	v.payload = nil
	C.krun_vmm_builder_devices(&v.vmmBuilder, v.manager)
	v.manager = nil
	if C.krun_check_nested_virt() {
		C.krun_vmm_builder_nested_virt(&v.vmmBuilder, true)
	}
	// Enable the upstream guest shutdown device. The request is still
	// best-effort on platforms where libkrun does not implement it.
	C.krun_vmm_builder_shutdown_support(&v.vmmBuilder, C.bool(true))
	var errOut C.KrunError
	v.vmm = C.krun_vmm_builder_build(&v.vmmBuilder, &errOut)
	v.vmmBuilder = nil
	if err := checkKrunHandle("build VMM", unsafe.Pointer(v.vmm), errOut); err != nil {
		return err
	}
	errOut = nil
	v.vmmHandle = C.krun_vmm_handle(v.vmm, &errOut)
	return checkKrunHandle("obtain VMM handle", unsafe.Pointer(v.vmmHandle), errOut)
}

func (v *Libkrun) freeGuestAgentData() {
	if v.guestAgentData != nil {
		C.free(v.guestAgentData)
		v.guestAgentData = nil
	}
}

type krunString struct {
	ptr   *C.char
	value C.KrunStr
}

func newKrunStr(s string) krunString {
	p := C.CString(s)
	return krunString{ptr: p, value: C.KrunStr{data: p, len: C.size_t(len(s))}}
}

func (s *krunString) free() {
	if s.ptr != nil {
		C.free(unsafe.Pointer(s.ptr))
		s.ptr = nil
	}
}

func initBuilderArg(builder *C.KrunInitBuilder, arg string) {
	s := newKrunStr(arg)
	defer s.free()
	C.krun_init_builder_arg(builder, s.value)
}

func checkKrunHandle(name string, handle unsafe.Pointer, errOut C.KrunError) error {
	if handle != nil {
		return nil
	}
	if errOut != nil {
		return formatKrunError(name, 0, errOut)
	}
	return fmt.Errorf("libkrun: %s failed", name)
}

func checkKrunResult(name string, result C.KrunResult, errOut C.KrunError) error {
	if result == C.KRUN_RESULT_SUCCESS {
		if errOut != nil {
			C.krun_error_destroy(errOut)
		}
		return nil
	}
	return formatKrunError(name, uint64(result), errOut)
}

func formatKrunError(name string, result uint64, errOut C.KrunError) error {
	if errOut == nil {
		return fmt.Errorf("libkrun: %s failed (result=%d)", name, result)
	}
	code := C.krun_error_code(errOut)
	message := C.krun_result_name_cstr(C.KrunResult(result))
	resultCode := C.krun_error_result(errOut)
	C.krun_error_destroy(errOut)
	return fmt.Errorf("libkrun: %s failed (result=%d code=%d error=%d name=%q)", name, result, uint32(code), uint64(resultCode), C.GoString(message))
}

func checkInitResult(name string, result C.KrunResult, errOut C.KrunInitError) error {
	if result == C.KRUN_RESULT_SUCCESS {
		if errOut != nil {
			C.krun_init_error_destroy(errOut)
		}
		return nil
	}
	if errOut == nil {
		return fmt.Errorf("libkrun: %s failed (result=%d)", name, uint64(result))
	}
	code := C.krun_init_error_code(errOut)
	resultCode := C.krun_init_error_result(errOut)
	C.krun_init_error_destroy(errOut)
	return fmt.Errorf("libkrun: %s failed (result=%d code=%d error=%d)", name, uint64(result), uint32(code), uint64(resultCode))
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
