//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../out/.deps/libkrun/include
#cgo LDFLAGS: -L ../../out/.deps/libkrun/lib/ -L../../out/.deps/libkrunfw/lib -lkrun -lkrunfw
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"linuxvm/pkg/httpserver"
	"linuxvm/pkg/interfaces"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

// cstring provides safe management of C string memory with automatic cleanup.
// It ensures that memory is freed even if a panic occurs.
//
// Example usage:
//
//	cs := newCString("hello")
//	defer cs.Free()
//	C.some_function(cs.Ptr())
type cstring struct {
	ptr *C.char
}

func newCString(s string) *cstring {
	return &cstring{ptr: C.CString(s)}
}

// Free releases the C string memory. Safe to call multiple times.
func (cs *cstring) Free() {
	if cs.ptr != nil {
		C.free(unsafe.Pointer(cs.ptr))
		cs.ptr = nil
	}
}

func (cs *cstring) Ptr() *C.char {
	return cs.ptr
}

type cstringArray struct {
	ptrs []*C.char
}

// newCStringArray creates a null-terminated array of C strings from Go strings.
// The caller MUST call Free() when done, typically via defer.
//
// cstringArray manages an array of C strings with automatic cleanup.
// The array is null-terminated as required by many C APIs.
//
// Example usage:
//
//	arr := newCStringArray([]string{"arg1", "arg2"})
//	defer arr.Free()
//	C.some_function(arr.Ptr())
func newCStringArray(strs []string) *cstringArray {
	// Allocate array with space for null terminator
	ptrs := make([]*C.char, len(strs)+1)
	for i, s := range strs {
		ptrs[i] = C.CString(s)
	}
	ptrs[len(strs)] = nil // Null terminator
	return &cstringArray{ptrs: ptrs}
}

// Free releases all C string memory. Safe to call multiple times.
func (csa *cstringArray) Free() {
	for i, ptr := range csa.ptrs {
		if ptr != nil {
			C.free(unsafe.Pointer(ptr))
			csa.ptrs[i] = nil
		}
	}
}

// Ptr returns a pointer to the first element, suitable for passing to C functions
// that expect a char** (null-terminated array of strings).
func (csa *cstringArray) Ptr() **C.char {
	if len(csa.ptrs) == 0 {
		return nil
	}
	return &csa.ptrs[0]
}

// VM resource limits
const (
	defaultNProcSoftLimit = 4096
	defaultNProcHardLimit = 8192
)

const (
	gpuFlagVenus   = 1 << 6 // Enable Venus (Vulkan passthrough)
	gpuFlagNoVirgl = 1 << 7 // Disable legacy VirGL (OpenGL)
)

// Default configurations
const (
	// defaultGPUFlags enables Vulkan passthrough without legacy OpenGL
	defaultGPUFlags = gpuFlagVenus | gpuFlagNoVirgl

	// defaultVirtIOFSMemoryWindow is 512MB for shared directory memory window
	defaultVirtIOFSMemoryWindow = 512 << 20
)

// vmState tracks the VM lifecycle state
type vmState int

const (
	// stateNew indicates the VM has been created but not yet configured
	stateNew vmState = iota
	// stateConfigured indicates Create() has completed successfully
	stateConfigured
	// stateRunning indicates Start() is executing
	stateRunning
	// stateStopped indicates the VM has stopped execution
	stateStopped
	// stateClosed indicates Close() has been called and resources are freed
	stateClosed
)

func (s vmState) String() string {
	switch s {
	case stateNew:
		return "new"
	case stateConfigured:
		return "configured"
	case stateRunning:
		return "running"
	case stateStopped:
		return "stopped"
	case stateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ConsolePortINOUT defines an I/O port on the virtio-console multiport device.
// The guest sees it as /dev/vportNpM and can identify it by name
// via /sys/class/virtio-ports/vportNpM/name.
type ConsolePortINOUT struct {
	Name     string
	InputFd  int // host → guest (host writes, guest reads)
	OutputFd int // guest → host (guest writes, host reads)
}

type LibkrunVM struct {
	vmc   *vmconfig.VMConfig
	ctxID uint32

	mu                sync.Mutex
	state             vmState
	consolePortsINOUT []ConsolePortINOUT
}

// guestMACAddress is the fixed MAC address for the guest VM network interface.
// to ensure the guest gets the expected IP (192.168.127.2).
var guestMACAddress = [6]byte{0x5a, 0x94, 0xef, 0xe4, 0x0c, 0xee}

// Compile-time check: LibkrunVM must implement vm.VMProvider
var _ interfaces.VMMProvider = (*LibkrunVM)(nil)

func NewLibkrunVM(vmc *vmconfig.VMConfig) *LibkrunVM {
	return &LibkrunVM{
		vmc:   vmc,
		state: stateNew,
	}
}

func (vm *LibkrunVM) AddConsolePort(port ConsolePortINOUT) *LibkrunVM {
	vm.consolePortsINOUT = append(vm.consolePortsINOUT, port)
	return vm
}

func (vm *LibkrunVM) GetVMConfigure() (*vmconfig.VMConfig, error) {
	if vm.vmc == nil {
		return nil, fmt.Errorf("vm configuration is nil")
	}
	return vm.vmc, nil
}

func (vm *LibkrunVM) Create(ctx context.Context) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state != stateNew {
		return fmt.Errorf("cannot create VM in state %s (must be in 'new' state)", vm.state)
	}

	// Initialize libkrun logging BEFORE creating context
	// This MUST be called before krun_create_ctx()
	if err := initLogging(); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	// Create libkrun context
	ctxID := C.krun_create_ctx()
	if ctxID < 0 {
		return fmt.Errorf("failed to create libkrun context: krun_create_ctx returned %d", ctxID)
	}
	vm.ctxID = uint32(ctxID)

	// Apply all VM configurations
	if err := vm.configureLibKRUN(ctx); err != nil {
		return fmt.Errorf("failed to configure VM: %w", err)
	}

	vm.state = stateConfigured
	return nil
}

// configureLibKRUN applies all VM configuration settings.
func (vm *LibkrunVM) configureLibKRUN(ctx context.Context) error {
	var err error

	// Phase 1: Core VM resources
	if err = vm.configureResources(); err != nil {
		return err
	}

	// Phase 2: Virtual devices (console, vsock, GPU)
	if err = vm.configureDevices(ctx); err != nil {
		return err
	}

	// Phase 3: Storage (rootfs, block devices, shared directories)
	if err = vm.configureStorage(); err != nil {
		return err
	}

	// Phase 4: Networking
	if err = vm.configureNetwork(ctx); err != nil {
		return err
	}

	// Phase 5: Advanced features
	if err = vm.configureAdvancedFeatures(); err != nil {
		return err
	}

	return nil
}

// configureResources sets CPU, memory, and resource limits.
func (vm *LibkrunVM) configureResources() error {
	cfg := vm.vmc

	ret := C.krun_set_vm_config(
		C.uint32_t(vm.ctxID),
		C.uint8_t(cfg.Cpus),
		C.uint32_t(cfg.MemoryInMB),
	)
	if ret != 0 {
		return fmt.Errorf("krun_set_vm_config failed with code %d", ret)
	}

	// Set guest process limits
	limitSpec := fmt.Sprintf("%d=%d:%d",
		process.RLIMIT_NPROC,
		defaultNProcSoftLimit,
		defaultNProcHardLimit,
	)
	limits := newCStringArray([]string{limitSpec})
	defer limits.Free()

	ret = C.krun_set_rlimits(C.uint32_t(vm.ctxID), limits.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_rlimits failed with code %d", ret)
	}

	return nil
}

func (vm *LibkrunVM) configureDevices(ctx context.Context) error {
	if err := vm.configureMultiportConsole(); err != nil {
		return err
	}

	ret := C.krun_disable_implicit_vsock(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_vsock failed with code %d", ret)
	}

	var vsockFeat = C.uint32_t(0)
	// TSI mode needs socket hijacking features
	if vm.vmc.VirtualNetworkMode == define.TSI.String() {
		// TSI mode
		if runtime.GOOS == "linux" {
			vsockFeat = C.KRUN_TSI_HIJACK_INET | C.KRUN_TSI_HIJACK_UNIX
		}
		// macOS do not support KRUN_TSI_HIJACK_UNIX
		//   see issue: https://github.com/containers/libkrun/issues/526
		if runtime.GOOS == "darwin" {
			vsockFeat = C.KRUN_TSI_HIJACK_INET
		}
	}

	ret = C.krun_add_vsock(C.uint32_t(vm.ctxID), vsockFeat)
	if ret != 0 {
		return fmt.Errorf("krun_add_vsock failed with code %d", ret)
	}

	ret = C.krun_set_gpu_options(C.uint32_t(vm.ctxID), C.uint32_t(defaultGPUFlags))
	if ret != 0 {
		return fmt.Errorf("krun_set_gpu_options failed with code %v", ret)
	}

	return nil
}

func (vm *LibkrunVM) configureMultiportConsole() error {
	ret := C.krun_disable_implicit_console(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_console failed with code %v", ret)
	}

	var isTTy bool
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stderr.Fd())) {
		isTTy = true
		// Setting TTY mode to true will force the guest-agent reopening the active console
		// provided full ioctl control of terminal like TIOCGWINSZ & TIOCSWINSZ
		vm.vmc.TTY = true
	}

	if !isTTy {
		// Setting TTY mode to false will prevent the guest-agent reopening the active console
		vm.vmc.TTY = false
		ret = C.krun_add_virtio_console_default(
			C.uint32_t(vm.ctxID),
			C.int(os.Stdin.Fd()),
			C.int(os.Stdout.Fd()),
			C.int(os.Stderr.Fd()),
		)
		if ret != 0 {
			return fmt.Errorf("krun_add_virtio_console_default failed with code %v", ret)
		}
	}

	consoleID := C.krun_add_virtio_console_multiport(C.uint32_t(vm.ctxID))
	if consoleID < 0 {
		return fmt.Errorf("krun_add_virtio_console_multiport failed with code %d", consoleID)
	}

	if isTTy {
		ttyFd, err := syscall.Dup(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("dup stdin: %w", err)
		}

		name := newCString(define.GuestTTYConsoleName)
		logrus.Infof("running in tty mode (stdin, stdout and stderr are all terminals)")
		ret := C.krun_add_console_port_tty(C.uint32_t(vm.ctxID), C.uint32_t(consoleID), name.Ptr(), C.int(ttyFd))
		name.Free()
		if ret != 0 {
			_ = syscall.Close(ttyFd)
			return fmt.Errorf("krun_add_console_port_tty failed with code %v", ret)
		}
	}

	// Do not close the guestLogFile.
	// TODO: guestLogFile my GC by golang, but it not real problem because the chance are so small
	guestLogFile, err := os.OpenFile(vm.vmc.GetVMMRunLogsFile(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	vm.consolePortsINOUT = append(vm.consolePortsINOUT, ConsolePortINOUT{
		Name:     define.GuestLogConsolePort,
		InputFd:  -1,
		OutputFd: int(guestLogFile.Fd()),
	})

	// additional in/out console
	for _, inoutConsole := range vm.consolePortsINOUT {
		name := newCString(inoutConsole.Name)
		ret := C.krun_add_console_port_inout(
			C.uint32_t(vm.ctxID),
			C.uint32_t(consoleID),
			name.Ptr(),
			C.int(inoutConsole.InputFd),
			C.int(inoutConsole.OutputFd))
		name.Free()
		if ret != 0 {
			return fmt.Errorf("krun_add_console_port_inout failed with code %v", ret)
		}
	}

	return nil
}

// configureStorage sets up rootfs, block devices, and shared directories.
func (vm *LibkrunVM) configureStorage() error {
	// Root filesystem
	runMode := vm.vmc.RunMode
	if runMode != define.ContainerMode.String() && runMode != define.RootFsMode.String() {
		return fmt.Errorf("unsupported run mode: %q (supported: %q, %q)",
			runMode, define.ContainerMode.String(), define.RootFsMode.String())
	}

	rootfs := newCString(vm.vmc.RootFS)
	defer rootfs.Free()

	ret := C.krun_set_root(C.uint32_t(vm.ctxID), rootfs.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_root failed with code %d", ret)
	}

	// Block devices
	for i, disk := range vm.vmc.BlkDevs {
		if err := vm.addBlockDevice(disk.Path); err != nil {
			return fmt.Errorf("failed to add block device %d (%q): %w", i, disk.Path, err)
		}
	}

	// Shared directories (VirtIO-FS)
	for i, mount := range vm.vmc.Mounts {
		if err := vm.addVirtIOFS(mount.Tag, mount.Source); err != nil {
			return fmt.Errorf("failed to add virtiofs mount %d (tag=%q): %w", i, mount.Tag, err)
		}
	}

	return nil
}

// configureNetwork sets up the network backend and VSock port mappings.
func (vm *LibkrunVM) configureNetwork(ctx context.Context) error {
	if vm.vmc.VirtualNetworkMode == define.GVISOR.String() {
		logrus.Infof("Using gvisor-tap-vsock network backend")
		// Parse network backend address
		backend := vm.vmc.GVPVNetAddr
		addr, err := network.ParseUnixAddr(backend)
		if err != nil {
			return fmt.Errorf("failed to parse network backend address %q: %w", backend, err)
		}

		socketPath := newCString(addr.Path)
		defer socketPath.Free()

		var mac [6]C.uint8_t
		for i, b := range guestMACAddress {
			mac[i] = C.uint8_t(b)
		}

		if ret := C.krun_add_net_unixgram(C.uint32_t(vm.ctxID), socketPath.Ptr(), C.int(-1), &mac[0],
			C.COMPAT_NET_FEATURES,
			C.NET_FLAG_VFKIT,
		); ret != 0 {
			return fmt.Errorf("krun_add_net_unixgram failed with code %v", ret)
		}
	} else {
		logrus.Infof("Using libkrun TSI network backend which out of box")
	}

	// VSock port mapping for ignition server
	ignAddr, err := network.ParseUnixAddr(vm.vmc.IgnitionServerCfg.ListenSockAddr)
	if err != nil {
		return fmt.Errorf("failed to parse ignition httpserver address: %w", err)
	}

	ignSocketPath := newCString(ignAddr.Path)
	defer ignSocketPath.Free()

	vsockPort := uint32(define.DefaultVSockPort)

	if ret := C.krun_add_vsock_port2(
		C.uint32_t(vm.ctxID),
		C.uint32_t(vsockPort),
		ignSocketPath.Ptr(),
		false,
	); ret != 0 {
		return fmt.Errorf("krun_add_vsock_port2 failed with code %v", ret)
	}
	logrus.Infof("VSock port %d mapped to ignition httpserver", vsockPort)

	return nil
}

// configureAdvancedFeatures enables optional features like nested virtualization.
func (vm *LibkrunVM) configureAdvancedFeatures() error {
	ret := C.krun_check_nested_virt()
	switch ret {
	case 0:
		return nil
	case 1:
		ret = C.krun_set_nested_virt(C.uint32_t(vm.ctxID), true)
		if ret != 0 {
			return fmt.Errorf("krun_set_nested_virt failed with code %d", ret)
		}
	default:
		return fmt.Errorf("krun_check_nested_virt failed with code %d", ret)
	}

	return nil
}

// initLogging initializes libkrun's logging subsystem.
// IMPORTANT: This MUST be called BEFORE krun_create_ctx().
// Do NOT call krun_set_log_level() if using this function - they conflict.
// TODO: save log into logs
func initLogging() error {
	var level C.uint32_t

	debugEnv := os.Getenv("LIBKRUN_DEBUG")
	switch debugEnv {
	case "trace":
		level = C.KRUN_LOG_LEVEL_TRACE
	case "debug", "1":
		level = C.KRUN_LOG_LEVEL_DEBUG
	case "info":
		level = C.KRUN_LOG_LEVEL_INFO
	case "warn":
		level = C.KRUN_LOG_LEVEL_WARN
	case "error":
		level = C.KRUN_LOG_LEVEL_ERROR
	default:
		level = C.KRUN_LOG_LEVEL_ERROR
	}

	ret := C.krun_init_log(
		C.int(C.KRUN_LOG_TARGET_DEFAULT),
		level,
		C.uint32_t(C.KRUN_LOG_STYLE_AUTO),
		C.KRUN_LOG_OPTION_NO_ENV,
	)
	if ret != 0 {
		return fmt.Errorf("krun_init_log failed with code %v", ret)
	}

	return nil
}

// addBlockDevice adds a single raw disk image to the VM.
func (vm *LibkrunVM) addBlockDevice(diskPath string) error {
	stat, err := os.Stat(diskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("disk image does not exist: %q", diskPath)
		}
		return fmt.Errorf("failed to stat disk %q: %w", diskPath, err)
	}

	if !stat.Mode().IsRegular() {
		return fmt.Errorf("disk path %q is not a regular file", diskPath)
	}

	diskID := newCString(uuid.New().String())
	defer diskID.Free()

	diskPathC := newCString(diskPath)
	defer diskPathC.Free()

	ret := C.krun_add_disk2(
		C.uint32_t(vm.ctxID),
		diskID.Ptr(),
		diskPathC.Ptr(),
		C.KRUN_DISK_FORMAT_RAW,
		false, // read-write
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_disk2 failed with code %v", ret)
	}

	return nil
}

// addVirtIOFS adds a single VirtIO-FS shared directory to the VM.
func (vm *LibkrunVM) addVirtIOFS(tag, hostPath string) error {
	absPath, err := filepath.Abs(hostPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %q: %w", hostPath, err)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks for %q: %w", absPath, err)
	}

	stat, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("host directory does not exist: %q", resolvedPath)
		}
		return fmt.Errorf("failed to stat directory %q: %w", resolvedPath, err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("host path %q is not a directory", resolvedPath)
	}

	tagC := newCString(tag)
	defer tagC.Free()

	pathC := newCString(resolvedPath)
	defer pathC.Free()

	ret := C.krun_add_virtiofs2(
		C.uint32_t(vm.ctxID),
		tagC.Ptr(),
		pathC.Ptr(),
		C.uint64_t(defaultVirtIOFSMemoryWindow),
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_virtiofs2 failed with code %d", ret)
	}

	return nil
}

// Start launches the VM and begins executing the configured command line.
// This blocks until the VM terminates or the context is cancelled.
func (vm *LibkrunVM) Start(ctx context.Context) error {
	vm.mu.Lock()
	vm.state = stateRunning
	defer vm.mu.Unlock()

	// Set host process resource limits
	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set host process resource limits: %w", err)
	}

	// Configure the command line to execute inside the VM
	if err := vm.applyGuestAgentCfg(); err != nil {
		return fmt.Errorf("failed to set VM command line: %w", err)
	}

	return vm.enterVMLifecycle(ctx)
}

// applyGuestAgentCfg configures the command, arguments, and environment for the guest.
//
// The first program executed by the virtual machine is `init`, provided by krun, and the second program is always the guest-agent.
func (vm *LibkrunVM) applyGuestAgentCfg() error {
	workdir := newCString(vm.vmc.GuestAgentCfg.Workdir)
	defer workdir.Free()

	if ret := C.krun_set_workdir(C.uint32_t(vm.ctxID), workdir.Ptr()); ret != 0 {
		return fmt.Errorf("krun_set_workdir failed with code %v", ret)
	}

	executable := newCString(define.GuestAgentPathInGuest)
	defer executable.Free()

	// guest-agent does not need any args, guest-agent read vmconfig to decide what to do
	args := newCStringArray(vm.vmc.GuestAgentCfg.Args)
	defer args.Free()

	// Set environment variables
	envs := newCStringArray(vm.vmc.GuestAgentCfg.Env)
	defer envs.Free()

	ret := C.krun_set_exec(
		C.uint32_t(vm.ctxID),
		executable.Ptr(),
		args.Ptr(),
		envs.Ptr(),
	)
	if ret != 0 {
		return fmt.Errorf("krun_set_exec failed with code %v", ret)
	}

	return nil
}

// enterVMLifecycle starts the VM and waits for it to terminate.
// This is the main VM execution loop that blocks until completion.
func (vm *LibkrunVM) enterVMLifecycle(ctx context.Context) error {
	errChan := make(chan error, 1)

	// Start VM in a goroutine so we can handle context cancellation
	go func() {
		ret := C.krun_start_enter(C.uint32_t(vm.ctxID))
		if ret != 0 {
			// Convert negative error code to errno for better error messages
			errno := syscall.Errno(-ret)
			errChan <- fmt.Errorf("VM execution failed: %w (libkrun code: %d)", errno, ret)
		} else {
			errChan <- nil
		}
	}()

	// Wait for either VM completion or context cancellation
	select {
	case <-ctx.Done():
		return fmt.Errorf("VM execution cancelled: %w", ctx.Err())
	case err := <-errChan:
		return err
	}
}

// Stop requests the VM to stop.
//
// Note: libkrun doesn't provide a graceful stop mechanism. so we have to implement a forceful shutdown
func (vm *LibkrunVM) Stop(_ context.Context) error {
	if vm.state != stateRunning {
		return nil
	}

	// TODO: implement STOP

	return nil
}

func (vm *LibkrunVM) StartIgnServer(ctx context.Context) error {
	return httpserver.NewIgnitionServer(vm.vmc).Start(ctx)
}
func (vm *LibkrunVM) StartVMCtlServer(ctx context.Context) error {
	return httpserver.NewManagementAPIServer(vm.vmc).Start(ctx)
}
