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
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	"linuxvm/pkg/system"
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

// Resource limits for the VM guest
const (
	// defaultNProcSoftLimit is the soft limit for number of processes
	defaultNProcSoftLimit = 4096
	// defaultNProcHardLimit is the hard limit for number of processes
	defaultNProcHardLimit = 8192
)

// VirtIO-GPU renderer flags (from virglrenderer.h)
const (
	virglUseEGL          = 1 << 0  // Use EGL for rendering
	virglThreadSync      = 1 << 1  // Enable thread synchronization
	virglUseGLX          = 1 << 2  // Use GLX for rendering
	virglUseSurfaceless  = 1 << 3  // Use surfaceless context
	virglUseGLES         = 1 << 4  // Use OpenGL ES
	virglUseExternalBlob = 1 << 5  // Use external blob resources
	virglVenus           = 1 << 6  // Enable Venus (Vulkan)
	virglNoVirgl         = 1 << 7  // Disable legacy VirGL
	virglUseAsyncFenceCB = 1 << 8  // Use async fence callbacks
	virglRenderServer    = 1 << 9  // Use render server
	virglDRM             = 1 << 10 // Use DRM
)

// Default GPU configuration: Venus (Vulkan) without legacy VirGL
const defaultGPUFlags = virglVenus | virglNoVirgl

// Default VirtIO-FS memory window size (512MB)
// This controls the memory window size for shared directories
const defaultVirtIOFSMemoryWindow = 1 << 29

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
	// config holds the VM configuration
	config *vmconfig.VMConfig

	// ctxID is the libkrun context identifier
	ctxID uint32

	// mu protects state transitions
	mu sync.Mutex

	// state tracks the current lifecycle state
	state vmState
}

// Ensure LibkrunVM implements vm.Provider interface
var _ interface {
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	StartNetwork(ctx context.Context) error
	GetVMConfigure() (*vmconfig.VMConfig, error)
} = (*LibkrunVM)(nil)

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
	logrus.Debug("starting network backend (gvproxy)")
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
// This is called internally by Create() and orchestrates all configuration steps.
func (vm *LibkrunVM) configureVM(ctx context.Context) error {
	// Set VM resources (CPU, memory)
	if err := vm.setResources(); err != nil {
		return fmt.Errorf("failed to set resources: %w", err)
	}

	// Set root filesystem
	if err := vm.setRootFS(); err != nil {
		return fmt.Errorf("failed to set root filesystem: %w", err)
	}

	// Set GPU options
	if err := vm.setGPU(); err != nil {
		return fmt.Errorf("failed to set GPU: %w", err)
	}

	// Configure explicit console (disable implicit and add our own)
	if err := vm.setConsole(); err != nil {
		return fmt.Errorf("failed to set console: %w", err)
	}

	// Set resource limits
	if err := vm.setResourceLimits(); err != nil {
		return fmt.Errorf("failed to set resource limits: %w", err)
	}

	// Set network provider
	if err := vm.setNetworkProvider(); err != nil {
		return fmt.Errorf("failed to set network provider: %w", err)
	}

	// Add block devices
	if err := vm.addBlockDevices(ctx); err != nil {
		return fmt.Errorf("failed to add block devices: %w", err)
	}

	// Add shared directories (VirtIO-FS)
	if err := vm.addSharedVirtioFsDirectories(); err != nil {
		return fmt.Errorf("failed to add shared directories: %w", err)
	}

	// Configure nested virtualization if available
	if err := vm.configureNestedVirt(ctx); err != nil {
		return fmt.Errorf("failed to configure nested virtualization: %w", err)
	}

	// Configure VSock for guest-host communication
	if err := vm.addVSockListener(ctx); err != nil {
		return fmt.Errorf("failed to add VSock listener: %w", err)
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

	logrus.Debugf("initialized libkrun logging with level %d", level)
	return nil
}

// setResources configures CPU and memory resources for the VM.
func (vm *LibkrunVM) setResources() error {
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

	return nil
}

// setRootFS configures the root filesystem for the VM.
func (vm *LibkrunVM) setRootFS() error {
	runMode := vm.config.RunMode

	// Validate run mode
	if runMode != define.ContainerMode.String() && runMode != define.RootFsMode.String() {
		return fmt.Errorf("libkrun does not support run mode: %q (supported: %q, %q)",
			runMode, define.ContainerMode.String(), define.RootFsMode.String())
	}

	rootfs := newCString(vm.config.RootFS)
	defer rootfs.Free()

	logrus.Infof("configuring root filesystem: %q (mode: %s)", vm.config.RootFS, runMode)
	ret := C.krun_set_root(C.uint32_t(vm.ctxID), rootfs.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_root failed with code %d", ret)
	}

	return nil
}

// setGPU configures GPU/graphics acceleration for the VM.
func (vm *LibkrunVM) setGPU() error {
	flags := C.uint32_t(defaultGPUFlags)
	logrus.Debug("configuring GPU: Venus (Vulkan) renderer, VirGL disabled")

	ret := C.krun_set_gpu_options(C.uint32_t(vm.ctxID), flags)
	if ret != 0 {
		return fmt.Errorf("krun_set_gpu_options failed with code %d", ret)
	}

	return nil
}

// setConsole configures explicit console for the VM.
// This disables the implicit console and adds an explicit virtio-console
// connected to the host's stdin/stdout/stderr.
func (vm *LibkrunVM) setConsole() error {
	// First, disable the implicit console so we have full control
	ret := C.krun_disable_implicit_console(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_console failed with code %d", ret)
	}
	logrus.Debug("disabled implicit console")

	// Add explicit virtio-console connected to host stdin/stdout/stderr
	// This creates a multi-port console that:
	// - Uses stdin for input to the guest
	// - Uses stdout for output from the guest
	// - Uses stderr for error output from the guest
	ret = C.krun_add_virtio_console_default(
		C.uint32_t(vm.ctxID),
		C.int(os.Stdin.Fd()),
		C.int(os.Stdout.Fd()),
		C.int(os.Stderr.Fd()),
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_virtio_console_default failed with code %d", ret)
	}

	logrus.Debugf("added explicit virtio-console (stdin=%d, stdout=%d, stderr=%d)",
		os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd())
	return nil
}

// setResourceLimits configures resource limits for processes inside the VM.
func (vm *LibkrunVM) setResourceLimits() error {
	// Format: "RLIMIT_TYPE=SOFT:HARD"
	// Using linux.RLIMIT_NPROC instead of hardcoded string "6"
	limitSpec := fmt.Sprintf("%d=%d:%d",
		process.RLIMIT_NPROC,
		defaultNProcSoftLimit,
		defaultNProcHardLimit,
	)

	limits := newCStringArray([]string{limitSpec})
	defer limits.Free()

	logrus.Debugf("configuring resource limits: NPROC soft=%d hard=%d",
		defaultNProcSoftLimit, defaultNProcHardLimit)

	ret := C.krun_set_rlimits(C.uint32_t(vm.ctxID), limits.Ptr())
	if ret != 0 {
		return fmt.Errorf("krun_set_rlimits failed with code %d", ret)
	}

	return nil
}

// Fixed MAC address for the guest VM network interface
// This MUST match the DHCP static lease in pkg/gvproxy/config.yaml
// to ensure the guest gets the expected IP (192.168.127.2)
var guestMACAddress = [6]C.uint8_t{0x5a, 0x94, 0xef, 0xe4, 0x0c, 0xee}

// setNetworkProvider configures the network backend (gvproxy) for the VM.
// Uses the new krun_add_net_unixgram API for explicit network device configuration.
func (vm *LibkrunVM) setNetworkProvider() error {
	backend := vm.config.NetworkStackBackend
	addr, err := network.ParseUnixAddr(backend)
	if err != nil {
		return fmt.Errorf("failed to parse network backend address %q: %w", backend, err)
	}

	socketPath := newCString(addr.Path)
	defer socketPath.Free()

	// Create a copy of the MAC address for the C call
	mac := guestMACAddress

	logrus.Infof("adding virtio-net device (unixgram): socket=%q, mac=%02x:%02x:%02x:%02x:%02x:%02x",
		addr.Path, mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])

	// Use krun_add_net_unixgram with:
	// - socketPath: path to gvproxy unix socket
	// - fd=-1: we're using socket path, not file descriptor
	// - mac: the guest's MAC address
	// - COMPAT_NET_FEATURES: standard network features for compatibility
	// - NET_FLAG_VFKIT: send VFKIT magic after connection (required by gvproxy in vfkit mode)
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

	return nil
}

// addBlockDevices adds all configured block devices to the VM.
func (vm *LibkrunVM) addBlockDevices(ctx context.Context) error {
	if len(vm.config.BlkDevs) == 0 {
		logrus.Debug("no block devices to add")
		return nil
	}

	logrus.Infof("adding %d block device(s)", len(vm.config.BlkDevs))
	for i, disk := range vm.config.BlkDevs {
		if err := vm.addRawDisk(disk.Path); err != nil {
			return fmt.Errorf("failed to add disk %d (%q): %w", i, disk.Path, err)
		}
	}
	return nil
}

// addRawDisk adds a single raw disk image to the VM.
func (vm *LibkrunVM) addRawDisk(diskPath string) error {
	// Verify disk exists before adding
	stat, err := os.Stat(diskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("disk image does not exist: %q", diskPath)
		}
		return fmt.Errorf("failed to stat disk %q: %w", diskPath, err)
	}

	// Validate it's a regular file
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("disk path %q is not a regular file", diskPath)
	}

	// Generate a unique ID for this disk
	diskID := newCString(uuid.New().String())
	defer diskID.Free()

	diskPathC := newCString(diskPath)
	defer diskPathC.Free()

	logrus.Infof("adding raw disk: %q (size: %d MB)", diskPath, stat.Size()/(1024*1024))
	ret := C.krun_add_disk2(
		C.uint32_t(vm.ctxID),
		diskID.Ptr(),
		diskPathC.Ptr(),
		C.KRUN_DISK_FORMAT_RAW,
		false, // read-write (not read-only)
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_disk2 failed with code %d", ret)
	}

	return nil
}

// addSharedVirtioFsDirectories adds all configured VirtIO-FS mounts to the VM.
func (vm *LibkrunVM) addSharedVirtioFsDirectories() error {
	if len(vm.config.Mounts) == 0 {
		logrus.Debug("no shared directories to add")
		return nil
	}

	logrus.Infof("adding %d shared director(ies)", len(vm.config.Mounts))
	for i, mount := range vm.config.Mounts {
		if err := vm.addVirtIOFS(mount.Tag, mount.Source); err != nil {
			return fmt.Errorf("failed to add VirtIO-FS mount %d (tag=%q, source=%q): %w",
				i, mount.Tag, mount.Source, err)
		}
	}
	return nil
}

// addVirtIOFS adds a single VirtIO-FS shared directory to the VM.
func (vm *LibkrunVM) addVirtIOFS(tag, hostPath string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(hostPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %q: %w", hostPath, err)
	}

	// Follow symlinks to get the real path
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks for %q: %w", absPath, err)
	}

	// Verify path exists and is a directory
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

	logrus.Infof("adding VirtIO-FS mount: %q → tag=%q", resolvedPath, tag)
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

// configureNestedVirt enables nested virtualization if supported by the host.
func (vm *LibkrunVM) configureNestedVirt(ctx context.Context) error {
	ret := C.krun_check_nested_virt()

	switch ret {
	case 0:
		logrus.Debug("nested virtualization not supported by host hardware, skipping")
		return nil
	case 1:
		logrus.Info("nested virtualization is supported by host hardware")
	default:
		// Non-zero, non-one return code indicates an error
		return fmt.Errorf("krun_check_nested_virt failed with code %d", ret)
	}

	// Enable nested virtualization
	ret = C.krun_set_nested_virt(C.uint32_t(vm.ctxID), true)
	if ret != 0 {
		return fmt.Errorf("failed to enable nested virtualization: krun_set_nested_virt returned %d", ret)
	}

	logrus.Info("enabled nested virtualization for guest")
	return nil
}

// addVSockListener configures VSock communication between host and guest.
// VSock is used for the ignition provisioner and other host-guest communication.
// This uses explicit VSock configuration (disables implicit vsock, then adds our own).
func (vm *LibkrunVM) addVSockListener(ctx context.Context) error {
	// First, disable implicit vsock to have full control
	ret := C.krun_disable_implicit_vsock(C.uint32_t(vm.ctxID))
	if ret != 0 {
		return fmt.Errorf("krun_disable_implicit_vsock failed with code %d", ret)
	}
	logrus.Debug("disabled implicit vsock")

	// Add explicit vsock device without TSI hijacking
	// TSI features:
	// - 0: No socket hijacking (explicit port mappings only)
	// - KRUN_TSI_HIJACK_INET: Hijack INET sockets
	// - KRUN_TSI_HIJACK_UNIX: Hijack UNIX sockets
	ret = C.krun_add_vsock(C.uint32_t(vm.ctxID), 0)
	if ret != 0 {
		return fmt.Errorf("krun_add_vsock failed with code %d", ret)
	}
	logrus.Debug("added explicit vsock device (no TSI hijacking)")

	// Add the ignition provisioner port mapping
	ignAddr := vm.config.IgnProvisionerAddr
	addr, err := network.ParseUnixAddr(ignAddr)
	if err != nil {
		return fmt.Errorf("failed to parse ignition server address %q: %w", ignAddr, err)
	}

	socketPath := newCString(addr.Path)
	defer socketPath.Free()

	vsockPort := uint32(define.DefaultVSockPort)
	logrus.Debugf("adding VSock port mapping: port=%d → unix_socket=%q", vsockPort, addr.Path)

	ret = C.krun_add_vsock_port2(
		C.uint32_t(vm.ctxID),
		C.uint32_t(vsockPort),
		socketPath.Ptr(),
		false, // false: guest initiates connection to host
	)
	if ret != 0 {
		return fmt.Errorf("krun_add_vsock_port2 failed with code %d", ret)
	}

	return nil
}

// Start launches the VM and begins executing the configured command line.
// This blocks until the VM terminates or the context is cancelled.
//
// This implements the vm.Provider interface.
func (vm *LibkrunVM) Start(ctx context.Context) error {
	vm.mu.Lock()
	if vm.state != stateConfigured {
		vm.mu.Unlock()
		return fmt.Errorf("cannot start VM in state %s (must be in 'configured' state)", vm.state)
	}
	vm.state = stateRunning
	vm.mu.Unlock()

	logrus.Info("starting VM execution")

	// Set host process resource limits
	// This increases the number of file descriptors and other limits for the host process
	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set host process resource limits: %w", err)
	}

	// Configure the command line to execute inside the VM
	if err := vm.setCommandLine(); err != nil {
		return fmt.Errorf("failed to set VM command line: %w", err)
	}

	// Start VM execution (blocks until VM exits)
	err := vm.executeVM(ctx)

	vm.mu.Lock()
	if err == nil {
		logrus.Debugf("VM execution finished (ctx_id=%d)", vm.ctxID)
		vm.state = stateStopped
	}
	vm.mu.Unlock()

	return err
}

// setCommandLine configures the command, arguments, and environment for the guest.
func (vm *LibkrunVM) setCommandLine() error {
	cmdline := vm.config.Cmdline

	// Set working directory
	workdir := newCString(cmdline.Workspace)
	defer workdir.Free()

	logrus.Debugf("setting working directory: %q", cmdline.Workspace)
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
		logrus.Debugf("passing %d environment variable(s) to guest", len(cmdline.Env))
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

// executeVM starts the VM and waits for it to terminate.
// This is the main VM execution loop that blocks until completion.
func (vm *LibkrunVM) executeVM(ctx context.Context) error {
	errChan := make(chan error, 1)

	// Start VM in a goroutine so we can handle context cancellation
	go func() {
		logrus.Debugf("entering VM execution loop (ctx_id=%d)", vm.ctxID)
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

// Stop gracefully stops the VM.
//
// Note: The current libkrun API doesn't provide a graceful stop mechanism.
// The VM will terminate when:
//   - The init process (guest agent) exits naturally
//   - The context passed to Start() is cancelled
//   - The host process exits
//
// This implements the vm.Provider interface.
func (vm *LibkrunVM) Stop(ctx context.Context) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	logrus.Infof("stop requested for VM (ctx_id=%d, state=%s)", vm.ctxID, vm.state)

	// Note: libkrun doesn't have an explicit stop/shutdown API.
	// The VM stops when the init process exits or the context is cancelled.

	if vm.state == stateRunning {
		logrus.Info("VM is running; it will stop when the init process exits")
	} else {
		logrus.Debugf("VM is not running (state: %s)", vm.state)
	}

	return nil
}

// Close releases all resources associated with the VM.
// This should be called when the VM is no longer needed.
//
// Note: In the current libkrun API, contexts are automatically cleaned up
// when the process exits. This method exists for interface compatibility
// and to mark the VM as closed.
func (vm *LibkrunVM) Close() error {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.state == stateClosed {
		logrus.Debug("VM already closed")
		return nil
	}

	logrus.Infof("closing VM (ctx_id=%d, state=%s)", vm.ctxID, vm.state)

	// Note: libkrun contexts are automatically cleaned up when the process exits.
	// There's no explicit krun_destroy_ctx function in the current API.
	// We mark the state as closed to prevent further operations.

	vm.state = stateClosed
	return nil
}

// SetKernel configures a custom kernel and initramfs for the VM.
// This is an advanced feature and is not typically needed for standard usage.
//
// Most users should use the default kernel embedded in the rootfs.
// This method is useful for kernel development or testing custom kernels.
func (vm *LibkrunVM) SetKernel(ctx context.Context) error {
	return vm.setKernel(
		vm.config.Kernel,
		vm.config.Initrd,
		vm.config.KernelCmdline...,
	)
}

// setKernel configures custom kernel, initramfs, and kernel command line.
func (vm *LibkrunVM) setKernel(kernelPath, initramfsPath string, cmdlineArgs ...string) error {
	// Validate kernel exists
	if exists, _ := filesystem.PathExists(kernelPath); !exists {
		return fmt.Errorf("kernel image not found: %q", kernelPath)
	}

	// Validate initramfs exists
	if exists, _ := filesystem.PathExists(initramfsPath); !exists {
		return fmt.Errorf("initramfs not found: %q", initramfsPath)
	}

	// Build command line string
	if len(cmdlineArgs) == 0 {
		return fmt.Errorf("kernel command line cannot be empty")
	}
	cmdline := strings.Join(cmdlineArgs, " ")

	kernel := newCString(kernelPath)
	defer kernel.Free()

	initramfs := newCString(initramfsPath)
	defer initramfs.Free()

	cmdlineC := newCString(cmdline)
	defer cmdlineC.Free()

	logrus.Infof("configuring custom kernel: %q", kernelPath)
	logrus.Infof("configuring initramfs: %q", initramfsPath)
	logrus.Debugf("kernel command line: %q", cmdline)

	ret := C.krun_set_kernel(
		C.uint32_t(vm.ctxID),
		kernel.Ptr(),
		C.KRUN_KERNEL_FORMAT_RAW,
		initramfs.Ptr(),
		cmdlineC.Ptr(),
	)
	if ret != 0 {
		return fmt.Errorf("krun_set_kernel failed with code %d", ret)
	}

	return nil
}

// NestVirt is an alias for configureNestedVirt for backward compatibility.
// Deprecated: Use configureNestedVirt internally or rely on Create() to configure it automatically.
func (vm *LibkrunVM) NestVirt(ctx context.Context) error {
	return vm.configureNestedVirt(ctx)
}
