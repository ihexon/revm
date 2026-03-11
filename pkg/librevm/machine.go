//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	commonlog "linuxvm/pkg/log"
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
				RunMode:    mode.String(),
				DiskXattrs: map[string]string{},
			},
			MachineRuntime: define.NewMachineRuntime(),
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

func (v *machineBuilder) setupLogLevel(level, customLogPath string) (*os.File, error) {
	logPath := filepath.Join(v.WorkspaceDir, "logs", "vm.log")
	if customLogPath != "" {
		absLogPath, err := filepath.Abs(filepath.Clean(customLogPath))
		if err != nil {
			return nil, err
		}
		logPath = absLogPath
	}
	v.LogFile = logPath

	f, err := commonlog.SetupLogger(level, "", v.LogFile)
	if err != nil {
		return nil, err
	}
	return f, nil
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

	if err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755); err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	helperGuestAgent := filepath.Join(filepath.Dir(execPath), "..", "helper", "guest-agent")
	guestAgentBytes, err := os.ReadFile(helperGuestAgent)
	if err != nil {
		return fmt.Errorf("failed to read guest-agent from %q: %w", helperGuestAgent, err)
	}

	if err := os.WriteFile(guestAgentFilePath, guestAgentBytes, 0755); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

func (v *machineBuilder) configurePodman(ctx context.Context) error {
	var envs []string

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
	keyPath := v.pathMgr.GetSSHPrivateKeyFile()
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
		HostSSHPublicKey:       string(publicKey),
		HostSSHPrivateKey:      string(privateKey),
		HostSSHPrivateKeyFile:  keyPath,
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
// The returned cleanup func releases all resources (file lock, log file,
// workspace directory). Caller must always invoke it.
func buildMachine(ctx context.Context, cfg Config, workspacePath string) (mc *define.Machine, cleanup func(), retErr error) {
	var runMode define.RunMode
	switch cfg.RunMode {
	case ModeRootfs:
		runMode = define.RootFsMode
	case ModeContainer:
		runMode = define.ContainerMode
	default:
		return nil, nil, fmt.Errorf("unsupported run mode %q", cfg.RunMode)
	}

	mBuilder := newMachineBuilder(runMode)

	cleanupCallbacks := system.NewCleanUp()
	cleanup = cleanupCallbacks.DoClean
	defer cleanupCallbacks.CleanIfErr(&retErr)

	if err := mBuilder.setupWorkspace(workspacePath); err != nil {
		return nil, nil, fmt.Errorf("setup workspace: %w", err)
	}
	cleanupCallbacks.AddFunc(func() { _ = mBuilder.fileLock.Unlock(); _ = os.Remove(workspacePath + ".lock") })
	cleanupCallbacks.AddFunc(func() { _ = os.RemoveAll(workspacePath) })

	logFile, err := mBuilder.setupLogLevel(cfg.LogLevel, cfg.LogTo)
	if err != nil {
		return nil, nil, fmt.Errorf("setup log level: %w", err)
	}

	cleanupCallbacks.AddFunc(func() { logrus.SetOutput(os.Stderr); _ = logFile.Close() })
	if err := mBuilder.configureSSH(); err != nil {
		return nil, nil, fmt.Errorf("generate ssh config: %w", err)
	}
	if err := mBuilder.withResources(cfg.MemoryMB, uint8(cfg.CPUs)); err != nil {
		return nil, nil, fmt.Errorf("set resources: %w", err)
	}
	if err := mBuilder.configureNetwork(ctx, define.VNetMode(cfg.Network)); err != nil {
		return nil, nil, fmt.Errorf("configure network: %w", err)
	}
	if cfg.Proxy {
		if err := mBuilder.applySystemProxy(); err != nil {
			return nil, nil, fmt.Errorf("apply system proxy: %w", err)
		}
	}

	if cfg.Rootfs != "" {
		if err := mBuilder.withUserProvidedRootfs(ctx, cfg.Rootfs); err != nil {
			return nil, nil, err
		}
	} else {
		if err := mBuilder.withBuiltInAlpineRootfs(ctx); err != nil {
			return nil, nil, err
		}
	}

	switch runMode {
	case define.RootFsMode:
		workDir := cfg.WorkDir
		if workDir == "" {
			workDir = "/"
		}
		bin := cfg.Command[0]
		var args []string
		if len(cfg.Command) > 1 {
			args = cfg.Command[1:]
		}
		if err := mBuilder.setupCmdLine(workDir, bin, args, cfg.Env); err != nil {
			return nil, nil, fmt.Errorf("setup cmdline: %w", err)
		}
	case define.ContainerMode:
		if err := mBuilder.withMountUserHome(ctx); err != nil {
			return nil, nil, fmt.Errorf("mount user home: %w", err)
		}
		if err := mBuilder.configurePodman(ctx); err != nil {
			return nil, nil, fmt.Errorf("configure podman: %w", err)
		}

		diskPath := mBuilder.pathMgr.GetBuiltInContainerStorageDiskFile()
		if cfg.ContainerDisk != "" {
			diskPath = cfg.ContainerDisk
		}
		if cfg.ContainerDiskVersion != "" {
			if err := mBuilder.resetOrReuseContainerRAWDisk(ctx, diskPath, cfg.ContainerDiskVersion); err != nil {
				return nil, nil, fmt.Errorf("check container disk version: %w", err)
			}
		}
		if err := mBuilder.configureContainerRAWDisk(ctx, diskPath); err != nil {
			return nil, nil, fmt.Errorf("setup container disk: %w", err)
		}
	}

	if len(cfg.Disks) > 0 {
		if err := mBuilder.withUserProvidedStorageRAWDisk(ctx, cfg.Disks); err != nil {
			return nil, nil, fmt.Errorf("attach raw disks: %w", err)
		}
	}
	if len(cfg.Mounts) > 0 {
		if err := mBuilder.withUserProvidedMounts(cfg.Mounts); err != nil {
			return nil, nil, fmt.Errorf("setup mounts: %w", err)
		}
	}
	if err := mBuilder.configureGuestAgent(ctx); err != nil {
		return nil, nil, fmt.Errorf("configure guest agent: %w", err)
	}
	if err := mBuilder.configureVMCtlAPI(); err != nil {
		return nil, nil, fmt.Errorf("configure vmctl API: %w", err)
	}

	// Detect TTY mode early so management API returns correct value
	mBuilder.detectTTY()

	return &mBuilder.Machine, cleanup, nil
}

func (v *machineBuilder) detectTTY() {
	v.TTY = term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		term.IsTerminal(int(os.Stderr.Fd()))
}

func getSessionDir(name string) string {
	return fmt.Sprintf("/tmp/%s", name)
}

func ignitionSockFile(workspace string) string {
	return filepath.Clean(filepath.Join(workspace, "socks", "ign.sock"))
}
