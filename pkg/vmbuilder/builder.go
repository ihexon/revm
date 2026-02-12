//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"strings"

	sysproxy "github.com/ihexon/getSysProxy"
	"github.com/sirupsen/logrus"
)

// VMConfigBuilder provides a fluent API for building VMConfig instances.
// Callers explicitly declare which features to enable via chain methods,
// and Build() applies them in the correct order.
type VMConfigBuilder struct {
	vmc     *VMConfig
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
}

// NewVMConfigBuilder creates a new builder for the specified run mode.
func NewVMConfigBuilder(runMode define.RunMode) *VMConfigBuilder {
	vmb := &VMConfigBuilder{
		vmc:     NewVMConfig(runMode),
		runMode: runMode,
	}

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

// Build constructs the VMConfig with all configurations applied in correct order.
func (b *VMConfigBuilder) Build(ctx context.Context) (*define.VMConfig, error) {
	// 1. Workspace
	if err := b.vmc.SetupWorkspace(b.workspace); err != nil {
		return nil, fmt.Errorf("setup workspace: %w", err)
	}

	// 2. Log level (needs workspace for log file path)
	if err := b.vmc.SetupLogLevel(b.logLevel); err != nil {
		return nil, fmt.Errorf("setup log level: %w", err)
	}

	b.pathMgr = NewPathManager(b.vmc.WorkspacePath)

	// 3. Resources
	if err := b.vmc.WithResources(b.memoryInMB, b.cpus); err != nil {
		return nil, fmt.Errorf("set resources: %w", err)
	}

	// 4. Network
	networkStrategy := GetNetworkStrategy(b.networkMode)
	if networkStrategy == nil {
		return nil, fmt.Errorf("invalid network mode: %s", b.networkMode)
	}

	b.vmc.VirtualNetworkMode = b.networkMode
	if err := networkStrategy.Configure(ctx, (*define.VMConfig)(b.vmc), b.pathMgr); err != nil {
		return nil, fmt.Errorf("configure network: %w", err)
	}

	if b.usingSystemProxy {
		// To simplify the code path, we only pick the http proxy from system proxy setting
		// the proxy must support CONNECT Method which can tunnel any NON HTTP Data
		httpProxy, err := sysproxy.GetHTTP()
		if err != nil {
			return nil, fmt.Errorf("get system proxy fail: %w", err)
		}

		ps := define.ProxySetting{
			Use: false,
		}

		if httpProxy == nil {
			logrus.Warnf("system proxy is not enabled")
		} else {
			// In GVISOR mode, localhost/127.0.0.1 refers host.containers.internal
			// which is the host address on virtualNetwork( which provided by gvisor-vsock-tap)
			if b.vmc.VirtualNetworkMode == define.GVISOR && (strings.Contains(httpProxy.String(), "127.0.0.1") ||
				strings.Contains(httpProxy.String(), "localhost")) {
				httpProxy.Host = define.HostDomainInGVPNet
			}

			ps.Use = true
			ps.HTTPSProxy = httpProxy.String()
			ps.HTTPProxy = httpProxy.String()
		}

		b.vmc.ProxySetting = ps
	}

	// 6. Rootfs
	if b.runMode == define.RootFsMode {
		if b.rootfsPath != "" {
			if err := b.vmc.WithUserProvidedRootfs(ctx, b.rootfsPath); err != nil {
				return nil, fmt.Errorf("setup custom rootfs: %w", err)
			}
		}
	}

	if b.builtInRootfs {
		if err := b.vmc.WithBuiltInAlpineRootfs(ctx); err != nil {
			return nil, fmt.Errorf("setup built-in rootfs: %w", err)
		}
	}

	// 7. Mount user home
	if b.runMode == define.ContainerMode || b.runMode == define.OVMode {
		if err := b.vmc.WithMountUserHome(ctx); err != nil {
			return nil, fmt.Errorf("mount user home: %w", err)
		}
	}

	// 8. Podman
	if b.runMode == define.ContainerMode || b.runMode == define.OVMode {
		podmanConfig := NewPodmanConfigurator(b.pathMgr)
		if err := podmanConfig.Configure(ctx, (*define.VMConfig)(b.vmc)); err != nil {
			return nil, fmt.Errorf("configure podman: %w", err)
		}
	}

	// 9. Container disk
	if b.runMode == define.OVMode {
		if err := b.vmc.ResetOrReuseContainerRAWDisk(ctx, b.containerDiskVersion); err != nil {
			return nil, fmt.Errorf("setup container disk: %w", err)
		}
	}

	if b.runMode == define.ContainerMode {
		if err := b.vmc.ConfigureContainerRAWDisk(ctx); err != nil {
			return nil, fmt.Errorf("setup container disk: %w", err)
		}
	}

	// 10. Raw disks
	if len(b.rawDisks) > 0 {
		if err := b.vmc.withUserProvidedStorageRAWDisk(ctx, b.rawDisks); err != nil {
			return nil, fmt.Errorf("attach raw disks: %w", err)
		}
	}

	// 11. User mounts
	if len(b.mounts) > 0 {
		if err := b.vmc.withUserProvidedMounts(b.mounts); err != nil {
			return nil, fmt.Errorf("setup mounts: %w", err)
		}
	}

	// 12. Cmdline
	if b.runMode == define.RootFsMode {
		if err := b.vmc.SetupCmdLine(b.workdir, b.bin, b.args, b.envs); err != nil {
			return nil, fmt.Errorf("setup cmdline: %w", err)
		}
	}

	// 13. Guest Agent (always required)
	guestAgentConfig := NewGuestAgentConfigurator(b.pathMgr)
	if err := guestAgentConfig.Configure(ctx, (*define.VMConfig)(b.vmc)); err != nil {
		return nil, fmt.Errorf("configure guest agent: %w", err)
	}

	// 14. VM Control API (always required)
	if err := b.vmc.configureVirtualMachineControlAPI(); err != nil {
		return nil, fmt.Errorf("configure vmctl API: %w", err)
	}

	return (*define.VMConfig)(b.vmc), nil
}
