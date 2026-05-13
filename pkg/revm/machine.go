//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	ssh "linuxvm/pkg/ssh"
	"linuxvm/pkg/static_resources"
	"linuxvm/pkg/system"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
	sysproxy "github.com/ihexon/getSysProxy"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

type machineBuilder struct {
	define.Machine
	fileLock *flock.Flock
	pathMgr  *machinePathManager
}

func newMachineBuilder(mode define.RunMode) *machineBuilder {
	return &machineBuilder{
		Machine: define.Machine{
			MachineSpec: define.MachineSpec{
				RunMode: mode.String(),
			},
		},
	}
}

func (v *machineBuilder) setupWorkspace(workspacePath string) error {
	if workspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	workspacePath, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	underTmp := strings.HasPrefix(workspacePath, "/tmp")
	underHome := strings.HasPrefix(workspacePath, homeDir)
	if !underTmp && !underHome {
		return fmt.Errorf("workspace must be under /tmp or home directory (%s), got %q", homeDir, workspacePath)
	}

	v.WorkspaceDir = workspacePath

	if err = os.MkdirAll(v.WorkspaceDir, 0755); err != nil {
		return err
	}

	v.pathMgr = newMachinePathManager(v.WorkspaceDir)

	return v.lock()
}

func (v *machineBuilder) lock() error {
	// Lock file lives OUTSIDE the workspace so that the clean helper can
	// acquire it after the workspace is deleted, preventing it from
	// removing a workspace that belongs to a new session with the same name.
	fileLock := flock.New(v.WorkspaceDir + ".lock")

	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("get lock failed: %w", err)
	}

	if !locked {
		return fmt.Errorf("session %q is locked by another instance", v.WorkspaceDir)
	}

	v.fileLock = fileLock
	return nil
}

func (v *machineBuilder) withResources(memoryInMB uint64, cpus uint8) error {
	if cpus == 0 {
		return fmt.Errorf("1 cpu cores is the minimum value")
	}

	if memoryInMB < 512 {
		return fmt.Errorf("512MB of memory is the minimum value")
	}

	v.MemoryInMB = memoryInMB
	v.Cpus = cpus

	return nil
}

func (v *machineBuilder) setupCmdLine(workdir, bin string, args, envs []string) error {
	if v.RunMode != define.RootFsMode.String() {
		return fmt.Errorf("expect run mode %q, but got %q", define.RootFsMode.String(), v.RunMode)
	}

	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	if workdir == "" {
		return fmt.Errorf("workdir path is empty")
	}

	if bin == "" {
		return fmt.Errorf("bin path is empty")
	}

	for _, arg := range args {
		if strings.Contains(arg, ";") || strings.Contains(arg, "|") ||
			strings.Contains(arg, "&") || strings.Contains(arg, "`") {
			return fmt.Errorf("dangerous shell metacharacters in argument: %s", arg)
		}
	}

	if v.ProxySetting.Use {
		envs = append(envs, "http_proxy="+v.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+v.ProxySetting.HTTPSProxy)
	}

	v.Cmdline = define.Cmdline{
		Bin:     bin,
		Args:    args,
		Envs:    envs,
		WorkDir: workdir,
	}

	return nil
}

func (v *machineBuilder) withBuiltInAlpineRootfs(ctx context.Context) error {
	if v.WorkspaceDir == "" {
		return fmt.Errorf("workspace path is empty")
	}

	alpineRootfsDir := v.pathMgr.GetRootfsDir()
	if err := os.MkdirAll(alpineRootfsDir, 0755); err != nil {
		return err
	}

	if err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsDir); err != nil {
		return err
	}

	return v.withUserProvidedRootfs(ctx, alpineRootfsDir)
}

func (v *machineBuilder) withUserProvidedRootfs(ctx context.Context, rootfsPath string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	rootfsPath, err := filepath.Abs(filepath.Clean(rootfsPath))
	if err != nil {
		return err
	}

	_, err = os.Lstat(filepath.Join(rootfsPath, "bin", "sh"))
	if err != nil {
		return fmt.Errorf("rootfs path %q does not contain shell interpreter /bin/sh: %w", rootfsPath, err)
	}

	v.RootFS = rootfsPath

	return nil
}

func (v *machineBuilder) withMountUserHome(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.withUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *machineBuilder) withUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return nil
	}

	mounts := filesystem.CmdLineMountToMounts(dirs)
	for i := range mounts {
		p, err := filepath.Abs(filepath.Clean(mounts[i].Source))
		if err != nil {
			return fmt.Errorf("failed to resolve mount source %q: %w", mounts[i].Source, err)
		}
		mounts[i].Source = p
	}
	v.Mounts = append(v.Mounts, mounts...)
	return nil
}

func (v *machineBuilder) configureGuestAgent(ctx context.Context) error {
	if v.WorkspaceDir == "" {
		return fmt.Errorf("workspace path is empty")
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   v.pathMgr.GetIgnSocketFile(),
	}

	if err := os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	if err := os.Remove(unixUSL.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	v.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenSockAddr: unixUSL.String(),
	}

	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	var finalEnv []string
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "TMPDIR=/tmp")
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomainInGVPNet))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	guestAgentFilePath := filepath.Join(v.RootFS, ".bin", "guest-agent")

	if err := static_resources.WriteEmbeddedGuestAgent(guestAgentFilePath); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

func (v *machineBuilder) configurePodman(ctx context.Context, userEnv []string) error {
	envs := append([]string{}, userEnv...)

	if v.ProxySetting.Use {
		envs = append(envs, "http_proxy="+v.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+v.ProxySetting.HTTPSProxy)
	}

	apiPath := v.pathMgr.GetPodmanSocketFile()

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   apiPath,
	}

	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}

	listenIP := define.UnspecifiedAddress
	if v.VirtualNetworkMode == define.TSI {
		listenIP = define.LocalHost
	}

	v.PodmanInfo = define.PodmanInfo{
		HostPodmanProxyAddr:      podmanProxyAddr.String(),
		GuestPodmanAPIListenAddr: net.JoinHostPort(listenIP, strconv.FormatUint(port, 10)),
		GuestPodmanRunWithEnvs:   envs,
	}

	if err := os.MkdirAll(filepath.Dir(podmanProxyAddr.Path), 0755); err != nil {
		return err
	}

	return nil
}

func (v *machineBuilder) configureSSH() error {
	keyPath := v.pathMgr.GetSSHKeyFilePath()
	pubKeyPath := keyPath + ".pub"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return err
	}

	privateKey, publicKey, err := ssh.GenerateKey()
	if err != nil {
		return err
	}
	if err = os.WriteFile(keyPath, privateKey, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(pubKeyPath, publicKey, 0644); err != nil {
		return err
	}

	v.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      string(publicKey),
		HostSSHPrivateKey:     string(privateKey),
		HostSSHPrivateKeyFile: keyPath,

		GuestSSHPrivateKeyFile: "/run/dropbear/private.key",
		GuestSSHAuthorizedKeys: "/run/dropbear/authorized_keys",
		GuestSSHPidFile:        "/run/dropbear/dropbear.pid",
	}

	return nil
}

func (v *machineBuilder) configureVMCtlAPI() error {
	apiPath := v.pathMgr.GetVMCtlSocketFile()

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   apiPath,
	}

	v.VMCtlAddr = unixAddr.String()

	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}
	if err := os.Remove(unixAddr.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// createSymlink creates a symlink at linkPath pointing to target.
// It ensures the parent directory of linkPath exists and removes any
// pre-existing file or symlink at linkPath.
func createSymlink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, linkPath)
}

func (v *machineBuilder) applySystemProxy() error {
	httpProxy, err := sysproxy.GetHTTP()
	if err != nil {
		return fmt.Errorf("get system proxy fail: %w", err)
	}

	if httpProxy == nil {
		logrus.Warnf("system proxy is not enabled, do nothing")
		return nil
	}

	if v.VirtualNetworkMode == define.GVISOR && (strings.Contains(httpProxy.String(), "127.0.0.1") ||
		strings.Contains(httpProxy.String(), "localhost")) {
		logrus.Debugf("in gvisor network mode, reset proxy to %s", define.HostDomainInGVPNet)
		httpProxy.Host = define.HostDomainInGVPNet
	}

	logrus.Infof("set http/https proxy to %s", httpProxy.String())
	v.ProxySetting = define.ProxySetting{
		Use:        true,
		HTTPProxy:  httpProxy.String(),
		HTTPSProxy: httpProxy.String(),
	}
	return nil
}

// --- Machine assembly (from Config) ----------------------------------------

// buildMachine converts Config directly into define.Machine.
// The returned cleanup func releases runtime resources (such as the session
// lock), but intentionally keeps the session workspace on disk. Caller must
// always invoke it.
func buildMachine(ctx context.Context, cfg Config, workspacePath string) (mc *define.Machine, cleanup func(), retErr error) {
	plan, err := newMachineBuildPlan(cfg, workspacePath)
	if err != nil {
		return nil, nil, err
	}
	defer plan.cleanupCallbacks.CleanIfErr(&retErr)

	if err := plan.build(ctx); err != nil {
		return nil, nil, err
	}

	return &plan.builder.Machine, plan.cleanupCallbacks.DoClean, nil
}

type machineBuildPlan struct {
	cfg              Config
	workspacePath    string
	runMode          define.RunMode
	builder          *machineBuilder
	cleanupCallbacks *system.CleanupCallback
}

func newMachineBuildPlan(cfg Config, workspacePath string) (*machineBuildPlan, error) {
	var runMode define.RunMode
	switch cfg.RunMode {
	case ModeRootfs:
		runMode = define.RootFsMode
	case ModeContainer:
		runMode = define.ContainerMode
	default:
		return nil, fmt.Errorf("unsupported run mode %q", cfg.RunMode)
	}

	return &machineBuildPlan{
		cfg:              cfg,
		workspacePath:    workspacePath,
		runMode:          runMode,
		builder:          newMachineBuilder(runMode),
		cleanupCallbacks: system.NewCleanUp(),
	}, nil
}

func (p *machineBuildPlan) build(ctx context.Context) error {
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{"workspace", p.setupWorkspace},
		{"logging", p.configureLogFile},
		{"ssh", p.configureSSH},
		{"resources", p.configureResources},
		{"network", p.configureNetwork},
		{"proxy", p.configureProxy},
		{"rootfs", p.prepareRootfs},
		{"mode", p.configureMode},
		{"storage", p.attachStorage},
		{"guest agent", p.configureGuestAgent},
		{"management API", p.configureManagementAPI},
		{"tty", p.detectTTY},
	}

	for _, step := range steps {
		if err := step.run(ctx); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func (p *machineBuildPlan) setupWorkspace(ctx context.Context) error {
	if err := p.builder.setupWorkspace(p.workspacePath); err != nil {
		return err
	}
	p.cleanupCallbacks.AddFunc(func() {
		_ = p.builder.fileLock.Unlock()
		_ = os.Remove(p.workspacePath + ".lock")
	})
	return nil
}

func (p *machineBuildPlan) configureLogFile(ctx context.Context) error {
	// cfg.LogTo is set by WithLogging; fall back to workspace-relative path.
	if p.cfg.LogTo != "" {
		p.builder.LogFile = p.cfg.LogTo
	} else {
		p.builder.LogFile = filepath.Join(p.builder.WorkspaceDir, "logs", "vm.log")
	}
	return nil
}

func (p *machineBuildPlan) configureSSH(ctx context.Context) error {
	return p.builder.configureSSH()
}

func (p *machineBuildPlan) configureResources(ctx context.Context) error {
	return p.builder.withResources(p.cfg.MemoryMB, uint8(p.cfg.CPUs))
}

func (p *machineBuildPlan) configureNetwork(ctx context.Context) error {
	return p.builder.configureNetwork(ctx, define.VNetMode(p.cfg.Network))
}

func (p *machineBuildPlan) configureProxy(ctx context.Context) error {
	if !p.cfg.Proxy {
		return nil
	}
	return p.builder.applySystemProxy()
}

func (p *machineBuildPlan) prepareRootfs(ctx context.Context) error {
	logrus.Info("preparing rootfs...")
	if p.cfg.Rootfs != "" {
		if err := p.builder.withUserProvidedRootfs(ctx, p.cfg.Rootfs); err != nil {
			return err
		}
	} else {
		if err := p.builder.withBuiltInAlpineRootfs(ctx); err != nil {
			return err
		}
	}
	logrus.Info("preparing rootfs completed")
	return nil
}

func (p *machineBuildPlan) configureMode(ctx context.Context) error {
	switch p.runMode {
	case define.RootFsMode:
		return p.configureRootfsMode(ctx)
	case define.ContainerMode:
		return p.configureContainerMode(ctx)
	default:
		return fmt.Errorf("unsupported run mode %q", p.runMode)
	}
}

func (p *machineBuildPlan) configureRootfsMode(ctx context.Context) error {
	workDir := p.cfg.WorkDir
	if workDir == "" {
		workDir = "/"
	}

	bin := p.cfg.Command[0]
	var args []string
	if len(p.cfg.Command) > 1 {
		args = p.cfg.Command[1:]
	}
	return p.builder.setupCmdLine(workDir, bin, args, p.cfg.Env)
}

func (p *machineBuildPlan) configureContainerMode(ctx context.Context) error {
	if p.builder.VirtualNetworkMode != define.GVISOR {
		return fmt.Errorf("container mode only supports %s network, got %s", define.GVISOR, p.builder.VirtualNetworkMode)
	}

	if err := p.builder.withMountUserHome(ctx); err != nil {
		return fmt.Errorf("mount user home: %w", err)
	}

	if err := p.builder.configurePodman(ctx, p.cfg.Env); err != nil {
		return fmt.Errorf("configure podman: %w", err)
	}

	logrus.Info("Preparing container storage disk...")
	if err := p.builder.configureContainerRAWDisk(ctx, p.cfg.ContainerDisk, p.builder.pathMgr.GetBuiltInContainerStorageDiskFile()); err != nil {
		return fmt.Errorf("setup container disk: %w", err)
	}

	return nil
}

func (p *machineBuildPlan) attachStorage(ctx context.Context) error {
	if len(p.cfg.Disks) > 0 {
		if err := p.builder.withUserProvidedStorageRAWDisk(ctx, p.cfg.Disks); err != nil {
			return fmt.Errorf("attach raw disks: %w", err)
		}
	}
	if len(p.cfg.Mounts) > 0 {
		if err := p.builder.withUserProvidedMounts(p.cfg.Mounts); err != nil {
			return fmt.Errorf("setup mounts: %w", err)
		}
	}
	return nil
}

func (p *machineBuildPlan) configureGuestAgent(ctx context.Context) error {
	return p.builder.configureGuestAgent(ctx)
}

func (p *machineBuildPlan) configureManagementAPI(ctx context.Context) error {
	return p.builder.configureVMCtlAPI()
}

func (p *machineBuildPlan) detectTTY(ctx context.Context) error {
	p.builder.detectTTY()
	return nil
}

func (v *machineBuilder) detectTTY() {
	v.TTY = term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		term.IsTerminal(int(os.Stderr.Fd()))
}

func getSessionDir(name string) string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", "revm", name)
	}
	return filepath.Join(dir, ".cache", "revm", name)
}

func ignitionSockFile(workspace string) string {
	return filepath.Clean(filepath.Join(workspace, "socks", "ign.sock"))
}
