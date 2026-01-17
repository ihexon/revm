//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L ../../out/lib/ -lkrun.1.17.0 -lkrunfw.5
#include <libkrun.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/sirupsen/logrus"
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

// newCString creates a new C string from a Go string.
// The caller MUST call Free() when done, typically via defer.
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

// Ptr returns the underlying C string pointer.
func (cs *cstring) Ptr() *C.char {
	return cs.ptr
}

// cstringArray manages an array of C strings with automatic cleanup.
// The array is null-terminated as required by many C APIs.
//
// Example usage:
//
//	arr := newCStringArray([]string{"arg1", "arg2"})
//	defer arr.Free()
//	C.some_function(arr.Ptr())
type cstringArray struct {
	ptrs []*C.char
}

// newCStringArray creates a null-terminated array of C strings from Go strings.
// The caller MUST call Free() when done, typically via defer.
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

// GPU configuration flags (from virglrenderer.h)
// Only defining flags that are actually used to avoid namespace pollution.
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

// LibkrunVM represents a libkrun virtual machine instance.
// It manages the complete lifecycle of a VM from creation through execution to cleanup.
//
// Lifecycle:
//  1. NewLibkrunVM() - Create new VM instance
//  2. StartNetwork() - Start network backend (gvproxy)
//  3. Create()       - Configure the VM
//  4. Start()        - Execute the VM (blocks until completion)
//  5. Stop()         - Stop the VM (optional, usually handled by context cancellation)
//  6. Close()        - Clean up resources
//
// Thread Safety:
// LibkrunVM is safe for concurrent method calls. Internal state is protected
// by a mutex, though typical usage is sequential (Create -> Start).
type LibkrunVM struct {
	config *vmconfig.VMConfig
	ctxID  uint32

	mu        sync.Mutex
	state     vmState
	closeOnce sync.Once
}

// guestMACAddress is the fixed MAC address for the guest VM network interface.
// This MUST match the DHCP static lease in pkg/gvproxy/config.yaml
// to ensure the guest gets the expected IP (192.168.127.2).
var guestMACAddress = [6]byte{0x5a, 0x94, 0xef, 0xe4, 0x0c, 0xee}

// Compile-time check: LibkrunVM must implement vm.Provider
var _ vm.Provider = (*LibkrunVM)(nil)

// NewLibkrunVM creates a new libkrun VM instance with the provided configuration.
//
// This function does not allocate any libkrun resources yet. Call Create()
// to actually configure the VM.
func NewLibkrunVM(cfg *vmconfig.VMConfig) *LibkrunVM {
	return &LibkrunVM{
		config: cfg,
		state:  stateNew,
	}
}

// GetVMConfigure returns the VM configuration.
// This implements the vm.Provider interface.
func (vm *LibkrunVM) GetVMConfigure() (*vmconfig.VMConfig, error) {
	if vm.config == nil {
		return nil, fmt.Errorf("vm configuration is nil")
	}
	return vm.config, nil
}

// StartNetwork starts the network backend (gvproxy) for the VM.
// This should be called before Create() to ensure the network is ready
// when the VM starts.
//
// This implements the vm.Provider interface.
func (vm *LibkrunVM) StartNetwork(ctx context.Context) error {
	logrus.Infof("starting network backend (gvproxy)")
	return gvproxy.Run(ctx, vm.config)
}

// Create configures the VM based on the provided configuration.
// This must be called before Start().
//
// This method:
//   - Creates the libkrun context
//   - Configures all VM resources (CPU, memory, disks, etc.)
//   - Sets up networking, GPU, and other devices
//   - Does NOT start the VM execution
//
// This implements the vm.Provider interface.
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

	logrus.Infof("created libkrun context with ID: %d", vm.ctxID)

	// Apply all VM configurations
	if err := vm.configureVM(ctx); err != nil {
		return fmt.Errorf("failed to configure VM: %w", err)
	}

	vm.state = stateConfigured
	logrus.Info("VM configuration completed successfully")
	return nil
}

// configureVM applies all VM configuration settings.
// Configuration is organized into logical phases for clarity.
func (vm *LibkrunVM) configureVM(ctx context.Context) error {
	var err error

	// Phase 1: Core VM resources
	if err = vm.configureResources(); err != nil {
		return err
	}

	// Phase 2: Virtual devices (console, vsock, GPU)
	if err = vm.configureDevices(); err != nil {
		return err
	}

	// Phase 3: Storage (rootfs, block devices, shared directories)
	if err = vm.configureStorage(); err != nil {
		return err
	}

	// Phase 4: Networking
	if err = vm.configureNetwork(); err != nil {
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
	cfg := vm.config
	logrus.Infof("configuring VM resources: %d MB memory, %d CPUs", cfg.MemoryInMB, cfg.Cpus)

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

	logrus.Infof("configuring resource limits: NPROC soft=%d hard=%d",
		defaultNProcSoftLimit, defaultNProcHardLimit)

	ret = C.krun_set_rlimits(C.uint32_t(vm.ctxID), limits.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_rlimits failed with code %d", ret)
	}

	return nil
}

// configureDevices sets up virtual devices: console, vsock, and GPU.
func (vm *LibkrunVM) configureDevices() error {
	// Console: disable implicit and add explicit
	ret := C.krun_disable_implicit_console(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_console failed with code %d", ret)
	}

	ret = C.krun_add_virtio_console_default(
		C.uint32_t(vm.ctxID),
		C.int(os.Stdin.Fd()),
		C.int(os.Stdout.Fd()),
		C.int(os.Stderr.Fd()),
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_virtio_console_default failed with code %d", ret)
	}
	logrus.Infof("configured virtio-console device")

	// VSock: disable implicit and add explicit
	ret = C.krun_disable_implicit_vsock(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_vsock failed with code %d", ret)
	}

	ret = C.krun_add_vsock(C.uint32_t(vm.ctxID), 0) // No TSI hijacking
	if ret != 0 {
		return fmt.Errorf("krun_add_vsock failed with code %d", ret)
	}
	logrus.Infof("configured vsock device")

	// GPU
	ret = C.krun_set_gpu_options(C.uint32_t(vm.ctxID), C.uint32_t(defaultGPUFlags))
	if ret != 0 {
		return fmt.Errorf("krun_set_gpu_options failed with code %d", ret)
	}
	logrus.Infof("configured GPU (Venus/Vulkan)")

	return nil
}

// configureStorage sets up rootfs, block devices, and shared directories.
func (vm *LibkrunVM) configureStorage() error {
	// Root filesystem
	runMode := vm.config.RunMode
	if runMode != define.ContainerMode.String() && runMode != define.RootFsMode.String() {
		return fmt.Errorf("unsupported run mode: %q (supported: %q, %q)",
			runMode, define.ContainerMode.String(), define.RootFsMode.String())
	}

	rootfs := newCString(vm.config.RootFS)
	defer rootfs.Free()

	logrus.Infof("configuring rootfs: %q (mode: %s)", vm.config.RootFS, runMode)
	ret := C.krun_set_root(C.uint32_t(vm.ctxID), rootfs.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_root failed with code %d", ret)
	}

	// Block devices
	for i, disk := range vm.config.BlkDevs {
		if err := vm.addBlockDevice(disk.Path); err != nil {
			return fmt.Errorf("failed to add block device %d (%q): %w", i, disk.Path, err)
		}
	}

	// Shared directories (VirtIO-FS)
	for i, mount := range vm.config.Mounts {
		if err := vm.addVirtIOFS(mount.Tag, mount.Source); err != nil {
			return fmt.Errorf("failed to add virtiofs mount %d (tag=%q): %w", i, mount.Tag, err)
		}
	}

	return nil
}

// configureNetwork sets up the network backend and VSock port mappings.
func (vm *LibkrunVM) configureNetwork() error {
	// Parse network backend address
	backend := vm.config.NetworkStackBackend
	addr, err := network.ParseUnixAddr(backend)
	if err != nil {
		return fmt.Errorf("failed to parse network backend address %q: %w", backend, err)
	}

	socketPath := newCString(addr.Path)
	defer socketPath.Free()

	// Convert Go byte array to C uint8_t array
	var mac [6]C.uint8_t
	for i, b := range guestMACAddress {
		mac[i] = C.uint8_t(b)
	}

	logrus.Infof("adding virtio-net: socket=%q, mac=%02x:%02x:%02x:%02x:%02x:%02x",
		addr.Path, mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])

	ret := C.krun_add_net_unixgram(
		C.uint32_t(vm.ctxID),
		socketPath.Ptr(),
		C.int(-1),
		&mac[0],
		C.COMPAT_NET_FEATURES,
		C.NET_FLAG_VFKIT,
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_net_unixgram failed with code %d", ret)
	}

	// VSock port mapping for ignition provisioner
	ignAddr, err := network.ParseUnixAddr(vm.config.VMConfigProvisionerAddr)
	if err != nil {
		return fmt.Errorf("failed to parse ignition server address: %w", err)
	}

	ignSocketPath := newCString(ignAddr.Path)
	defer ignSocketPath.Free()

	vsockPort := uint32(define.DefaultVSockPort)
	logrus.Infof("adding vsock port mapping: port=%d → %q", vsockPort, ignAddr.Path)

	ret = C.krun_add_vsock_port2(
		C.uint32_t(vm.ctxID),
		C.uint32_t(vsockPort),
		ignSocketPath.Ptr(),
		false,
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_vsock_port2 failed with code %d", ret)
	}

	return nil
}

// configureAdvancedFeatures enables optional features like nested virtualization.
func (vm *LibkrunVM) configureAdvancedFeatures() error {
	ret := C.krun_check_nested_virt()
	switch ret {
	case 0:
		logrus.Infof("nested virtualization not supported, skipping")
		return nil
	case 1:
		ret = C.krun_set_nested_virt(C.uint32_t(vm.ctxID), true)
		if ret != 0 {
			return fmt.Errorf("krun_set_nested_virt failed with code %d", ret)
		}
		logrus.Info("enabled nested virtualization")
	default:
		return fmt.Errorf("krun_check_nested_virt failed with code %d", ret)
	}

	return nil
}

// initLogging initializes libkrun's logging subsystem.
// IMPORTANT: This MUST be called BEFORE krun_create_ctx().
// Do NOT call krun_set_log_level() if using this function - they conflict.
func initLogging() error {
	var level C.uint32_t

	// Map logrus levels to libkrun levels
	switch logrus.GetLevel() {
	case logrus.TraceLevel:
		level = C.KRUN_LOG_LEVEL_TRACE
	case logrus.DebugLevel:
		level = C.KRUN_LOG_LEVEL_DEBUG
	case logrus.InfoLevel:
		level = C.KRUN_LOG_LEVEL_INFO
	case logrus.WarnLevel:
		level = C.KRUN_LOG_LEVEL_WARN
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		level = C.KRUN_LOG_LEVEL_ERROR
	default:
		level = C.KRUN_LOG_LEVEL_INFO
	}

	// Use krun_init_log with:
	// - KRUN_LOG_TARGET_DEFAULT (-1): write to stderr
	// - level: the log level we determined above
	// - KRUN_LOG_STYLE_AUTO: auto-detect terminal color support
	// - KRUN_LOG_OPTION_NO_ENV: don't allow env vars to override these settings
	ret := C.krun_init_log(
		C.KRUN_LOG_TARGET_DEFAULT,
		level,
		C.KRUN_LOG_STYLE_AUTO,
		C.KRUN_LOG_OPTION_NO_ENV,
	)
	if ret != 0 {
		return fmt.Errorf("krun_init_log failed with code %d", ret)
	}

	logrus.Infof("initialized libkrun logging with level %d", level)
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

	logrus.Infof("adding block device: %q (size: %d MB)", diskPath, stat.Size()/(1024*1024))
	ret := C.krun_add_disk2(
		C.uint32_t(vm.ctxID),
		diskID.Ptr(),
		diskPathC.Ptr(),
		C.KRUN_DISK_FORMAT_RAW,
		false, // read-write
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_disk2 failed with code %d", ret)
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

	logrus.Infof("adding virtio-fs: %q → tag=%q", resolvedPath, tag)
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
	if vm.state != stateConfigured {
		vm.mu.Unlock()
		return fmt.Errorf("cannot start VM in state %s (must be 'configured')", vm.state)
	}
	vm.state = stateRunning
	vm.mu.Unlock()

	logrus.Info("starting VM execution")

	// Set host process resource limits
	if err := system.Rlimit(); err != nil {
		vm.mu.Lock()
		vm.state = stateConfigured // Restore state on failure
		vm.mu.Unlock()
		return fmt.Errorf("failed to set host process resource limits: %w", err)
	}

	// Configure the command line to execute inside the VM
	if err := vm.setCommandLine(); err != nil {
		vm.mu.Lock()
		vm.state = stateConfigured // Restore state on failure
		vm.mu.Unlock()
		return fmt.Errorf("failed to set VM command line: %w", err)
	}

	// Start VM execution (blocks until VM exits)
	err := vm.enterVMLifecycle(ctx)

	vm.mu.Lock()
	vm.state = stateStopped
	vm.mu.Unlock()

	return err
}

// setCommandLine configures the command, arguments, and environment for the guest.
func (vm *LibkrunVM) setCommandLine() error {
	cmdline := vm.config.Cmdline

	// Set working directory
	workdir := newCString(cmdline.Workspace)
	defer workdir.Free()

	logrus.Infof("setting working directory: %q", cmdline.Workspace)
	ret := C.krun_set_workdir(C.uint32_t(vm.ctxID), workdir.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_workdir failed with code %d", ret)
	}

	// Set executable (guest agent binary)
	executable := newCString(cmdline.GuestAgent)
	defer executable.Free()

	// Set arguments to pass to the guest agent
	args := newCStringArray(cmdline.GuestAgentArgs)
	defer args.Free()

	// Set environment variables
	envs := newCStringArray(cmdline.Env)
	defer envs.Free()

	logrus.Infof("configuring guest-agent: %q (args: %v)", cmdline.GuestAgent, cmdline.GuestAgentArgs)
	if len(cmdline.Env) > 0 {
		logrus.Infof("passing %d environment variable(s) to guest", len(cmdline.Env))
	}

	ret = C.krun_set_exec(
		C.uint32_t(vm.ctxID),
		executable.Ptr(),
		args.Ptr(),
		envs.Ptr(),
	)
	if ret != 0 {
		return fmt.Errorf("krun_set_exec failed with code %d", ret)
	}

	return nil
}

// enterVMLifecycle starts the VM and waits for it to terminate.
// This is the main VM execution loop that blocks until completion.
func (vm *LibkrunVM) enterVMLifecycle(ctx context.Context) error {
	errChan := make(chan error, 1)

	// Start VM in a goroutine so we can handle context cancellation
	go func() {
		logrus.Infof("entering VM execution loop (ctx_id=%d)", vm.ctxID)
		ret := C.krun_start_enter(C.uint32_t(vm.ctxID))
		if ret != 0 {
			// Convert negative error code to errno for better error messages
			errno := syscall.Errno(-ret)
			errChan <- fmt.Errorf("VM execution failed: %w (libkrun code: %d)", errno, ret)
		} else {
			logrus.Info("VM execution completed successfully")
			errChan <- nil
		}
	}()

	// Wait for either VM completion or context cancellation
	select {
	case <-ctx.Done():
		// Context cancelled - VM might still be running
		// Note: libkrun doesn't provide a clean shutdown mechanism
		// The VM process will be forcefully terminated when the host process exits
		logrus.Warn("VM execution cancelled by context")
		return fmt.Errorf("VM execution cancelled: %w", ctx.Err())
	case err := <-errChan:
		return err
	}
}

// Stop requests the VM to stop.
//
// Note: libkrun doesn't provide a graceful stop mechanism.
// The VM terminates when the init process exits or context is cancelled.
func (vm *LibkrunVM) Stop(_ context.Context) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	logrus.Infof("stop requested for VM (ctx_id=%d, state=%s)", vm.ctxID, vm.state)
	return nil
}

// Close releases all resources associated with the VM.
// Safe to call multiple times.
func (vm *LibkrunVM) Close() error {
	vm.closeOnce.Do(func() {
		vm.mu.Lock()
		defer vm.mu.Unlock()

		logrus.Infof("closing VM (ctx_id=%d, state=%s)", vm.ctxID, vm.state)
		vm.state = stateClosed
	})
	return nil
}