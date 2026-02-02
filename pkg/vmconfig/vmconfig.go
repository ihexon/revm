//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	_ "embed"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/logger"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/static_resources"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type (
	VMConfig define.VMConfig
)

func (v *VMConfig) Lock() error {
	fileLock := flock.New(filepath.Join(v.WorkspacePath, ".lock"))

	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("get lock failed: %w", err)
	}

	if !ifLocked {
		return fmt.Errorf("file %q is locked by another vm instance", fileLock.Path())
	}

	return nil
}

func (v *VMConfig) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManagerHost()
	if err != nil {
		return err
	}

	if err = static_resources.ExtractEmbeddedRawDisk(ctx, rawDiskPath); err != nil {
		return err
	}

	if err = diskMgr.NewUUID(ctx, givenUUID, rawDiskPath); err != nil {
		return err
	}

	return nil
}

func (v *VMConfig) GetContainerStorageDiskPath() string {
	return filepath.Join(v.WorkspacePath, "raw-disk", "container-storage.ext4")
}

func (v *VMConfig) AutoAttachContainerStorageRawDisk(ctx context.Context) error {
	f := v.GetContainerStorageDiskPath()
	_, err := os.Stat(f)
	if err != nil {
		logrus.Infof("try to generate container storage raw disk: %q", f)
		if err = v.generateRAWDisk(ctx, f, define.ContainerDiskUUID); err != nil {
			return fmt.Errorf("failed to generate container storage raw disk: %w", err)
		}
	}

	return v.attachRAWDisk(ctx, f)
}

func (v *VMConfig) attachRAWDisk(ctx context.Context, rawDiskPath string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManagerHost()
	if err != nil {
		return err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return err
	}

	// raw disk with ContainerDiskUUID should mount to ContainerStorageMountPoint
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

func (v *VMConfig) BindUserHomeDir(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logrus.Infof("in container mode, directory %q will be auto mount to guest %q", homeDir, homeDir)
	return v.WithMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VMConfig) WithGivenRAWDisk(ctx context.Context, rawDiskS []string) error {
	for _, f := range rawDiskS {
		if f == "" {
			return fmt.Errorf("raw disk path is empty")
		}

		rawDiskPath, err := filepath.Abs(filepath.Clean(f))
		if err != nil {
			return err
		}

		if _, err = os.Stat(rawDiskPath); err == nil {
			if err = v.attachRAWDisk(ctx, rawDiskPath); err != nil {
				return err
			}
		} else {
			if err = v.generateRAWDisk(ctx, rawDiskPath, uuid.NewString()); err != nil {
				return err
			}
			if err = v.attachRAWDisk(ctx, rawDiskPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (v *VMConfig) SetupCmdLine(workdir, bin string, args, envs []string, usingSystemProxy bool) error {
	if v.RunMode != define.RootFsMode.String() {
		return fmt.Errorf("expect run mode %q, but got %q", define.RootFsMode.String(), v.RunMode)
	}

	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	if workdir == "" {
		workdir = "/"
	}

	if filepath.Clean(bin) == "" {
		return fmt.Errorf("bin path is empty")
	}

	for _, arg := range args {
		if strings.Contains(arg, ";") || strings.Contains(arg, "|") ||
			strings.Contains(arg, "&") || strings.Contains(arg, "`") {
			return fmt.Errorf("dangerous shell metacharacters in argument: %s", arg)
		}
	}

	if usingSystemProxy {
		logrus.Warnf("your system proxy must support CONNECT method")
		proxyInfo, err := network.GetAndNormalizeSystemProxy()
		if err != nil {
			return fmt.Errorf("failed to get and normalize system proxy: %w", err)
		}

		if proxyInfo.HTTP != nil {
			httpProxy := fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port)
			logrus.Infof("using http proxy: %q", httpProxy)
			envs = append(envs, httpProxy)
		}

		if proxyInfo.HTTPS != nil {
			httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
			logrus.Infof("using https proxy: %q", httpsProxy)
			envs = append(envs, httpsProxy)
		}
	}

	v.Cmdline = define.Cmdline{
		Bin:     bin,
		Args:    args,
		Envs:    envs,
		WorkDir: workdir,
	}

	return nil
}

// SetupGuestAgentCfg must be called after the rootfs is set up
func (v *VMConfig) SetupGuestAgentCfg() error {
	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	// inject user-given envs to guest-agent
	var finalEnv []string
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "TMPDIR=/tmp")

	// In the virtualNetwork, HOST_DOMAIN is the domain name of the host, and the guest can access network resources on the host through HOST_DOMAIN.
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomain))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	guestAgentFilePath := filepath.Join(v.RootFS, ".bin", "guest-agent")

	if err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(guestAgentFilePath, static_resources.GuestAgentBytes, 0755); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

func (v *VMConfig) WithResources(memoryInMB uint64, cpus int8) error {
	if cpus <= 0 {
		return fmt.Errorf("1 cpu cores is the minimum value")
	}

	if memoryInMB < 512 {
		return fmt.Errorf("512MB of memory is the minimum value")
	}

	v.MemoryInMB = memoryInMB
	v.Cpus = cpus

	return nil
}

func (v *VMConfig) WithMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return nil
	}

	var hostDirs []string
	for _, dir := range dirs {
		p, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		hostDirs = append(hostDirs, p)
	}

	v.Mounts = append(v.Mounts, filesystem.CmdLineMountToMounts(hostDirs)...)
	return nil
}

// SetupIgnitionServerCfg extracts the guest-agent into the rootfs and sets up the ignition unix-socket listening address
func (v *VMConfig) SetupIgnitionServerCfg() error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	port, err := network.GetAvailablePort(62234)
	if err != nil {
		return err
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   v.GetSocksPath("ign.sock"),
	}

	tcpURL := &url.URL{
		Scheme: "tcp",
		Host:   net.JoinHostPort(define.LocalHost, strconv.Itoa(int(port))),
	}

	if err = os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	_ = os.Remove(unixUSL.Path)

	v.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenTcpAddr:      tcpURL.String(),
		ListenUnixSockAddr: unixUSL.String(),
	}

	return nil
}

func (v *VMConfig) SetLogLevel(level string, logFile string) error {
	logrus.SetOutput(os.Stderr)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logrus.SetLevel(l)

	maxSizeBytes := int64(filesystem.MiB(10).ToBytes())
	if logFile != "" {
		logFile, err = filepath.Abs(logFile)
		if err != nil {
			return err
		}
		logFile = filepath.Clean(logFile)
		// Check file size and truncate if > 100MB
		if info, err := os.Stat(logFile); err == nil && info.Size() > maxSizeBytes {
			if err = os.Truncate(logFile, 0); err != nil {
				logrus.Warnf("failed to truncate log file: %v", err)
			}
			logrus.Infof("log file truncated: %s", logFile)
		}

		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logrus.SetOutput(f)
			logger.LogFd = f
			logrus.Infof("log file: %s", logFile)
		}
	}

	return nil
}

func (v *VMConfig) WithBuiltInAlpineRootfs(ctx context.Context) error {
	alpineRootfsPath := filepath.Join(v.WorkspacePath, "rootfs")
	if err := os.MkdirAll(alpineRootfsPath, 0755); err != nil {
		return err
	}

	if err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsPath); err != nil {
		return err
	}

	return v.WithRootfs(ctx, alpineRootfsPath)
}

func (v *VMConfig) WithRootfs(ctx context.Context, rootfsPath string) error {
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

	// flock do not need release
	ok, err := flock.New(filepath.Join(v.RootFS, ".lock")).TryLock()
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("rootfs %q is locked by another instance", rootfsPath)
	}

	return nil
}

func (v *VMConfig) WithRunMode(runMode define.RunMode) {
	v.RunMode = runMode.String()
}

func (v *VMConfig) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "socks", name))
}

func (v *VMConfig) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "ssh", "key"))
}

func (v *VMConfig) withPodmanCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetSocksPath("podman-listen.sock"),
	}

	v.PodmanInfo = define.PodmanInfo{
		LocalPodmanProxyAddr: unixAddr.String(),
		GuestPodmanAPIIP:     define.GuestIP,
		GuestPodmanAPIPort:   define.GuestPodmanAPIPort,
	}

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) withVMControlCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetSocksPath("vm-control.sock"),
	}

	v.VMCtlAddress = unixAddr.String()

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) withGvisorTapVsockCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetSocksPath("gvpctl.sock"),
	}

	v.GvisorTapVsockEndpoint = unixAddr.String()
	v.GvisorTapVsockNetwork = fmt.Sprintf("unixgram://%s", v.GetSocksPath("gvpnet.sock"))

	_ = os.Remove(v.GetSocksPath("gvpctl.sock"))
	_ = os.Remove(v.GetSocksPath("gvpnet.sock"))

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) generateSSHCfg() error {
	keyPair, err := ssh.GenerateKeyPair(v.GetSSHPrivateKeyFile(), ssh.DefaultKeyGenOptions())
	if err != nil {
		return err
	}

	v.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      keyPair.AuthorizedKey(),
		HostSSHPrivateKey:     string(keyPair.RawProtectedPrivateKey()),
		HostSSHPrivateKeyFile: v.GetSSHPrivateKeyFile(),
	}

	if err := os.MkdirAll(filepath.Dir(v.SSHInfo.HostSSHPrivateKeyFile), 0700); err != nil {
		return err
	}

	_ = os.Remove(v.SSHInfo.HostSSHPrivateKeyFile)
	_ = os.Remove(v.SSHInfo.HostSSHPrivateKeyFile + ".pub")

	return keyPair.WriteKeys()
}

func (v *VMConfig) SetupWorkspace(workspacePath string) error {
	if workspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	workspacePath, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return err
	}

	v.WorkspacePath = workspacePath

	if err = os.MkdirAll(v.WorkspacePath, 0755); err != nil {
		return err
	}

	// first we lock the workspace, only one vm instance can access the workspace at a time
	err = v.Lock()
	if err != nil {
		return err
	}
	logrus.Infof("workspace locked, current running instance pid: %d", os.Getpid())

	if v.RunMode == define.ContainerMode.String() {
		if err = v.withPodmanCfg(); err != nil {
			return err
		}
		logrus.Infof("podman api proxy addr: %q", v.PodmanInfo.LocalPodmanProxyAddr)
	}

	if err = v.withVMControlCfg(); err != nil {
		return err
	}
	logrus.Infof("vm control API listen in: %q", v.VMCtlAddress)

	if err = v.withGvisorTapVsockCfg(); err != nil {
		return err
	}
	logrus.Infof("gvisor-tap-vsock control endpoint: %q, network: %q", v.GvisorTapVsockEndpoint, v.GvisorTapVsockNetwork)

	if err = v.generateSSHCfg(); err != nil {
		return err
	}
	logrus.Infof("ssh key pair generated in: %q", v.SSHInfo.HostSSHPrivateKeyFile)

	return nil
}

func NewVMConfig(mode define.RunMode) *VMConfig {
	vmc := &VMConfig{
		RunMode: mode.String(),
	}

	return vmc
}
