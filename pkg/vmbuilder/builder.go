//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
)

// VMConfigBuilder provides a fluent API for building VM instances.
// Callers explicitly declare which features to enable via chain methods,
// and Build() applies them in the correct order.
type VMConfigBuilder struct {
	vmc     *VM
	pathMgr *PathManager

	// Configuration parameters (Set*)
	runMode          define.RunMode
	networkMode      define.VNetMode
	workspace        string
	logLevel         string
	cpus             int8
	memoryInMB       uint64
	usingSystemProxy bool
	rawDisks         []string
	mounts           []string
	rootfsPath       string

	// Cmdline parameters
	workdir string
	bin     string
	args    []string
	envs    []string

	// Feature flags (With*)
	builtInRootfs        bool
	containerDiskVersion string
	containerDiskPath    string
}

// NewVMConfigBuilder creates a new builder for the specified run mode.
func NewVMConfigBuilder(runMode define.RunMode) *VMConfigBuilder {
	vmb := &VMConfigBuilder{runMode: runMode}

	if runMode == define.ContainerMode || runMode == define.OVMode {
		vmb.builtInRootfs = true
	}

	return vmb
}

// --- Configuration parameters ---

// SetWorkspace sets the workspace path.
func (b *VMConfigBuilder) SetWorkspace(path string) *VMConfigBuilder {
	b.workspace = path
	return b
}

// SetLogLevel sets the log level.
func (b *VMConfigBuilder) SetLogLevel(level string) *VMConfigBuilder {
	b.logLevel = level
	return b
}

// SetResources sets CPU and memory resources.
func (b *VMConfigBuilder) SetResources(cpus int8, memoryInMB uint64) *VMConfigBuilder {
	b.cpus = cpus
	b.memoryInMB = memoryInMB
	return b
}

// SetNetworkMode sets the virtual network mode (GVISOR or TSI).
func (b *VMConfigBuilder) SetNetworkMode(mode define.VNetMode) *VMConfigBuilder {
	b.networkMode = mode
	return b
}

// SetUsingSystemProxy enables system proxy configuration.
func (b *VMConfigBuilder) SetUsingSystemProxy(enable bool) *VMConfigBuilder {
	b.usingSystemProxy = enable
	return b
}

// SetRawDisks sets the list of raw disk paths to attach.
func (b *VMConfigBuilder) SetRawDisks(disks []string) *VMConfigBuilder {
	b.rawDisks = disks
	return b
}

// SetMounts sets the list of host directories to mount.
func (b *VMConfigBuilder) SetMounts(mounts []string) *VMConfigBuilder {
	b.mounts = mounts
	return b
}

// SetRootfs sets a custom rootfs path. Takes priority over WithBuiltInRootfs.
func (b *VMConfigBuilder) SetRootfs(path string) *VMConfigBuilder {
	b.rootfsPath = path
	b.builtInRootfs = false
	return b
}

// SetCmdline sets the command to execute inside the VM.
func (b *VMConfigBuilder) SetCmdline(workdir, bin string, args, envs []string) *VMConfigBuilder {
	b.workdir = workdir
	b.bin = bin
	b.args = args
	b.envs = envs
	return b
}

// --- Feature opt-ins ---

// WithBuiltInRootfs enables the built-in Alpine rootfs.
func (b *VMConfigBuilder) WithBuiltInRootfs() *VMConfigBuilder {
	b.builtInRootfs = true
	b.rootfsPath = ""
	return b
}

func (b *VMConfigBuilder) SetContainerDiskVersion(version string) *VMConfigBuilder {
	b.containerDiskVersion = version
	return b
}

func (b *VMConfigBuilder) SetContainerDiskPath(path string) *VMConfigBuilder {
	b.containerDiskPath = path
	return b
}

// Build constructs the VM with all configurations applied in correct order.
func (b *VMConfigBuilder) Build(ctx context.Context) (*define.Machine, error) {
	b.vmc = NewVirtualMachine(b.runMode)

	// ── Common setup (all modes) ─────────────────────────────────────────

	if err := b.vmc.setupWorkspace(b.workspace); err != nil {
		return nil, fmt.Errorf("setup workspace: %w", err)
	}
	b.pathMgr = NewPathManager(b.vmc.WorkspacePath)

	if err := b.vmc.configureSSH(b.pathMgr); err != nil {
		return nil, fmt.Errorf("generate ssh config: %w", err)
	}
	if err := b.vmc.setupLogLevel(b.logLevel); err != nil {
		return nil, fmt.Errorf("setup log level: %w", err)
	}
	if err := b.vmc.withResources(b.memoryInMB, b.cpus); err != nil {
		return nil, fmt.Errorf("set resources: %w", err)
	}
	if err := b.vmc.configureNetwork(ctx, b.networkMode, b.pathMgr); err != nil {
		return nil, fmt.Errorf("configure network: %w", err)
	}
	if b.usingSystemProxy {
		if err := b.vmc.applySystemProxy(); err != nil {
			return nil, fmt.Errorf("apply system proxy: %w", err)
		}
	}

	// ── Rootfs (all modes) ──────────────────────────────────────────────

	var rootfsErr error
	if b.rootfsPath != "" {
		rootfsErr = b.vmc.withUserProvidedRootfs(ctx, b.rootfsPath)
	} else {
		rootfsErr = b.vmc.withBuiltInAlpineRootfs(ctx, b.pathMgr)
	}

	if rootfsErr != nil {
		return nil, rootfsErr
	}

	// ── Mode-specific setup ──────────────────────────────────────────────

	switch b.runMode {
	case define.RootFsMode:
		if err := b.vmc.setupCmdLine(b.workdir, b.bin, b.args, b.envs); err != nil {
			return nil, fmt.Errorf("setup cmdline: %w", err)
		}

	case define.ContainerMode:
		if err := b.vmc.withMountUserHome(ctx); err != nil {
			return nil, fmt.Errorf("mount user home: %w", err)
		}
		if err := b.vmc.configurePodman(ctx, b.pathMgr); err != nil {
			return nil, fmt.Errorf("configure podman: %w", err)
		}

		diskPath := b.pathMgr.GetBuiltInContainerStorageDiskPathInWorkspace()
		// if user given --container-disk, set diskPath to user given path
		if b.containerDiskPath != "" {
			diskPath = b.containerDiskPath
		}

		if err := b.vmc.configureContainerRAWDisk(ctx, diskPath); err != nil {
			return nil, fmt.Errorf("setup container disk: %w", err)
		}
	case define.OVMode:
		if err := b.vmc.withMountUserHome(ctx); err != nil {
			return nil, fmt.Errorf("mount user home: %w", err)
		}
		if err := b.vmc.configurePodman(ctx, b.pathMgr); err != nil {
			return nil, fmt.Errorf("configure podman: %w", err)
		}
		diskPath := b.pathMgr.GetBuiltInContainerStorageDiskPathInWorkspace()
		if err := b.vmc.resetOrReuseContainerRAWDisk(ctx, diskPath, b.containerDiskVersion); err != nil {
			return nil, fmt.Errorf("setup container disk: %w", err)
		}
	}

	// ── Common finalization (all modes) ──────────────────────────────────

	if len(b.rawDisks) > 0 {
		if err := b.vmc.withUserProvidedStorageRAWDisk(ctx, b.rawDisks); err != nil {
			return nil, fmt.Errorf("attach raw disks: %w", err)
		}
	}
	if len(b.mounts) > 0 {
		if err := b.vmc.withUserProvidedMounts(b.mounts); err != nil {
			return nil, fmt.Errorf("setup mounts: %w", err)
		}
	}
	if err := b.vmc.configureGuestAgent(ctx, b.pathMgr); err != nil {
		return nil, fmt.Errorf("configure guest agent: %w", err)
	}
	if err := b.vmc.configureVMCtlAPI(b.pathMgr); err != nil {
		return nil, fmt.Errorf("configure vmctl API: %w", err)
	}

	return &b.vmc.Machine, nil
}
