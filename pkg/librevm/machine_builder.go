//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	sshv2 "linuxvm/pkg/ssh_v2"
	"linuxvm/pkg/static_resources"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	sysproxy "github.com/ihexon/getSysProxy"
	"github.com/sirupsen/logrus"
)

type machineBuilder struct {
	define.Machine
	fileLock *flock.Flock
}

func newMachineBuilder(mode define.RunMode) *machineBuilder {
	return &machineBuilder{
		Machine: define.Machine{
			MachineSpec: define.MachineSpec{
				RunMode:       mode.String(),
				XATTRSRawDisk: map[string]string{},
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

	v.WorkspacePath = workspacePath

	if err = os.MkdirAll(v.WorkspacePath, 0755); err != nil {
		return err
	}

	return v.lock()
}

func (v *machineBuilder) lock() error {
	// Lock file lives OUTSIDE the workspace so that the clean helper can
	// acquire it after the workspace is deleted, preventing it from
	// removing a workspace that belongs to a new session with the same name.
	fileLock := flock.New(v.WorkspacePath + ".lock")

	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("get lock failed: %w", err)
	}

	if !locked {
		return fmt.Errorf("session %q is locked by another instance", v.WorkspacePath)
	}

	v.fileLock = fileLock
	return nil
}

func (v *machineBuilder) setupLogLevel(level string) (*os.File, error) {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	v.LogFilePath = filepath.Join(v.WorkspacePath, "logs", "vm.log")

	if err := os.MkdirAll(filepath.Dir(v.LogFilePath), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	if info, err := os.Stat(v.LogFilePath); err == nil && info.Size() > int64(filesystem.MiB(10).ToBytes()) {
		_ = os.Truncate(v.LogFilePath, 0)
	}

	f, err := os.OpenFile(v.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))

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

// machinePathManager handles all workspace-relative path calculations.
type machinePathManager struct {
	workspacePath string
}

func newMachinePathManager(workspacePath string) *machinePathManager {
	return &machinePathManager{workspacePath: workspacePath}
}

func (p *machinePathManager) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(p.workspacePath, "socks", name))
}

func (p *machinePathManager) GetPodmanListenAddr() string {
	return p.GetSocksPath("podman-api.sock")
}

func (p *machinePathManager) GetVNetListenAddr() string {
	return p.GetSocksPath("vnet.sock")
}

func (p *machinePathManager) GetGVPCtlAddr() string {
	return p.GetSocksPath("gvpctl.sock")
}

func (p *machinePathManager) GetVMCtlAddr() string {
	return p.GetSocksPath("vmctl.sock")
}

func (p *machinePathManager) GetIgnAddr() string {
	return p.GetSocksPath("ign.sock")
}

func (p *machinePathManager) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(p.workspacePath, "ssh", "key"))
}

func (p *machinePathManager) GetLogsDir() string {
	return filepath.Join(p.workspacePath, "logs")
}

func (p *machinePathManager) GetRootfsDir() string {
	return filepath.Join(p.workspacePath, "rootfs")
}

func (p *machinePathManager) GetBuiltInContainerStorageDiskPathInWorkspace() string {
	return filepath.Join(p.workspacePath, "raw-disk", "container-storage.ext4")
}

func (v *machineBuilder) withBuiltInAlpineRootfs(ctx context.Context, pathMgr *machinePathManager) error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	alpineRootfsDir := pathMgr.GetRootfsDir()
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

// networkConfigStrategy defines the interface for network configuration strategies.
// Different network modes (GVISOR, TSI) implement this interface to configure
// the VM's network stack in their specific way.
type networkConfigStrategy interface {
	// Configure sets up network configuration on the given VM.
	// pathMgr is used to get workspace-relative paths for socket files.
	Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error
}

func (v *machineBuilder) configureNetwork(ctx context.Context, mode define.VNetMode, pathMgr *machinePathManager) error {
	strategy := getNetworkStrategy(mode)
	if strategy == nil {
		return fmt.Errorf("invalid network mode: %s", mode)
	}
	v.VirtualNetworkMode = mode
	return strategy.Configure(ctx, &v.Machine, pathMgr)
}

// getNetworkStrategy returns the appropriate network strategy for the given network mode.
// Returns nil if the mode is invalid/unknown.
func getNetworkStrategy(mode define.VNetMode) networkConfigStrategy {
	switch mode {
	case define.GVISOR:
		return &gVisorNetworkConfig{}
	case define.TSI:
		return &tsiNetworkConfig{}
	default:
		return nil
	}
}

// gVisorNetworkConfig implements network configuration for gvisor-tap-vsock mode.
// This mode uses gvisor's userspace network stack with vsock communication.
type gVisorNetworkConfig struct{}

// Configure sets up the gvisor-tap-vsock network configuration.
// It creates Unix socket paths for GVProxy control and virtual network communication.
func (g *gVisorNetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error {
	logrus.Infof("Configuring gvisor-tap-vsock network mode")

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetGVPCtlAddr(),
	}

	vmc.GVPCtlAddr = unixAddr.String()

	// On Linux, use unix:// (stream socket for QemuProtocol).
	// On macOS, use unixgram:// (datagram socket for VfkitProtocol).
	if runtime.GOOS == "linux" {
		vmc.GVPVNetAddr = fmt.Sprintf("unix://%s", pathMgr.GetVNetListenAddr())
	} else {
		vmc.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetListenAddr())
	}

	// Clean up any existing sockets
	_ = os.Remove(pathMgr.GetGVPCtlAddr())
	_ = os.Remove(pathMgr.GetVNetListenAddr())

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}

	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	vmc.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.UnspecifiedAddress, strconv.FormatUint(port, 10))
	return nil
}

// tsiNetworkConfig implements network configuration for TSI (Transparent Socket Interception) mode.
// TSI mode uses libkrun's built-in network capabilities without external network stack.
type tsiNetworkConfig struct{}

// Configure sets up TSI network mode.
// TSI mode doesn't require gvisor network setup, but we record the host-accessible
// SSH address since guest ports are directly reachable via libkrun.
func (t *tsiNetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error {
	logrus.Infof("Using TSI network mode (libkrun built-in networking)")
	// TSI: guest port is directly accessible on host via libkrun
	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	vmc.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.LocalHost, strconv.FormatUint(port, 10))
	vmc.SSHInfo.HostSSHProxyListenAddr = vmc.SSHInfo.GuestSSHServerListenAddr
	return nil
}

func (v *machineBuilder) configureGuestAgent(ctx context.Context, pathMgr *machinePathManager) error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   pathMgr.GetIgnAddr(),
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

func (v *machineBuilder) configurePodman(ctx context.Context, pathMgr *machinePathManager) error {
	var envs []string

	if v.ProxySetting.Use {
		envs = append(envs, "http_proxy="+v.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+v.ProxySetting.HTTPSProxy)
	}

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetPodmanListenAddr(),
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

func (v *machineBuilder) configureSSH(pathMgr *machinePathManager) error {
	keyPath := pathMgr.GetSSHPrivateKeyFile()
	pubKeyPath := keyPath + ".pub"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return err
	}

	privateKey, publicKey, err := sshv2.GenerateKey()
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
		GuestSSHPrivateKeyPath: "/run/dropbear/private.key",
		GuestSSHAuthorizedKeys: "/run/dropbear/authorized_keys",
		GuestSSHPidFile:        "/run/dropbear/dropbear.pid",
	}

	return nil
}

func (v *machineBuilder) configureVMCtlAPI(pathMgr *machinePathManager) error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetVMCtlAddr(),
	}

	v.VMCtlAddress = unixAddr.String()

	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}
	if err := os.Remove(unixAddr.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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

func (v *machineBuilder) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	if err = static_resources.ExtractEmbeddedRawDisk(ctx, rawDiskPath); err != nil {
		return fmt.Errorf("failed to extract embedded raw disk: %w", err)
	}

	if err = diskMgr.NewUUID(ctx, givenUUID, rawDiskPath); err != nil {
		return fmt.Errorf("failed to write UUID for raw disk %q: %w", rawDiskPath, err)
	}

	xattrWriter := filesystem.NewXATTRManager()

	for xattrKind, xattrValue := range v.XATTRSRawDisk {
		if err = xattrWriter.WriteXATTR(ctx, rawDiskPath, xattrKind, xattrValue, true); err != nil {
			return fmt.Errorf("failed to write xattr %s to %s: %w", xattrKind, rawDiskPath, err)
		}
	}

	return nil
}

func (v *machineBuilder) configureContainerRAWDisk(ctx context.Context, diskPath string) error {
	if _, err := os.Stat(diskPath); err != nil {
		if err = v.generateRAWDisk(ctx, diskPath, define.ContainerDiskUUID); err != nil {
			return fmt.Errorf("failed to generate container storage raw disk: %w", err)
		}
	}

	return v.addRAWDiskToBlkList(ctx, diskPath)
}

func (v *machineBuilder) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return err
	}

	if info.UUID == define.ContainerDiskUUID {
		info.MountTo = define.ContainerStorageMountPoint
	}

	blkDev := define.BlkDev{
		UUID:    info.UUID,
		FsType:  info.FsType,
		Path:    info.Path,
		MountTo: info.MountTo,
	}

	v.BlkDevs = append(v.BlkDevs, blkDev)

	return nil
}

func (v *machineBuilder) withUserProvidedStorageRAWDisk(ctx context.Context, rawDiskS []string) error {
	for _, f := range rawDiskS {
		if f == "" {
			return fmt.Errorf("raw disk path is empty")
		}

		rawDiskPath, err := filepath.Abs(filepath.Clean(f))
		if err != nil {
			return err
		}
		if _, err = os.Stat(rawDiskPath); err != nil {
			if err = v.generateRAWDisk(ctx, rawDiskPath, uuid.NewString()); err != nil {
				return err
			}
		}

		if err = v.addRAWDiskToBlkList(ctx, rawDiskPath); err != nil {
			return err
		}
	}

	return nil
}

//nolint:unused
func (v *machineBuilder) resetOrReuseContainerRAWDisk(ctx context.Context, diskPath string, containerDiskVersionXATTR string) error {
	resetBool, err := v.withRAWDiskVersionXATTR(containerDiskVersionXATTR).needsDiskRegeneration(ctx, diskPath)
	if err != nil {
		return fmt.Errorf("failed to check RAW disk needs to regenerate: %w", err)
	}

	if resetBool {
		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			return err
		}

		if err := v.configureContainerRAWDisk(ctx, diskPath); err != nil {
			return fmt.Errorf("failed to attach container storage raw disk: %w", err)
		}
	}

	return nil
}

//nolint:unused
func (v *machineBuilder) needsDiskRegeneration(ctx context.Context, diskPath string) (bool, error) {
	xattrKey := define.XATTRRawDiskVersionKey
	xattrProcesser := filesystem.NewXATTRManager()

	value1, _ := xattrProcesser.GetXATTR(ctx, diskPath, xattrKey)
	value2 := v.XATTRSRawDisk[xattrKey]
	if value2 == "" {
		return false, fmt.Errorf("vmc XATTRSRawDisk not set")
	}

	if value1 != value2 {
		return true, nil
	}

	return false, nil
}

//nolint:unused
func (v *machineBuilder) withRAWDiskVersionXATTR(value string) *machineBuilder {
	v.XATTRSRawDisk = map[string]string{
		define.XATTRRawDiskVersionKey: value,
	}
	return v
}
