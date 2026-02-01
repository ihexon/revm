//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/logger"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/static_resources"
	"net/url"
	"os"
	"path/filepath"
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

	logrus.Infof("try to lock file: %q", fileLock.Path())

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

	err = static_resources.ExtractEmbeddedRawDisk(ctx, rawDiskPath)
	if err != nil {
		return err
	}

	err = diskMgr.NewUUID(ctx, givenUUID, rawDiskPath)
	if err != nil {
		return err
	}

	return v.attachRAWDisk(ctx, rawDiskPath)
}

func (v *VMConfig) GetContainerStorageDiskPath() string {
	return filepath.Join(v.WorkspacePath, "raw-disk", "container-storage.ext4")
}

func (v *VMConfig) AutoAttachContainerStorageRawDisk(ctx context.Context) error {
	f := v.GetContainerStorageDiskPath()
	_, err := os.Stat(f)
	if err == nil {
		return v.attachRAWDisk(ctx, f)
	}

	return v.generateRAWDisk(ctx, f, define.ContainerDiskUUID)
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
		}
	}

	return nil
}

func validateArgs(args []string) error {
	for _, arg := range args {
		if strings.Contains(arg, ";") || strings.Contains(arg, "|") ||
			strings.Contains(arg, "&") || strings.Contains(arg, "`") {
			return fmt.Errorf("dangerous shell metacharacters in argument: %s", arg)
		}
	}
	return nil
}

func (v *VMConfig) RunCmdline(workdir, bin string, args, envs []string, usingSystemProxy bool) error {

	if filepath.Clean(workdir) == "" {
		workdir = "/"
	}

	if strings.TrimSpace(bin) == "" {
		return fmt.Errorf("target binary is empty")
	}

	if err := validateArgs(args); err != nil {
		return err
	}

	// inject user-given envs
	var finalEnv []string
	finalEnv = append(finalEnv, envs...)
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "HOME=/root")
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	if usingSystemProxy {
		proxyInfo, err := network.GetAndNormalizeSystemProxy()
		if err != nil {
			return fmt.Errorf("failed to get and normalize system proxy: %w", err)
		}

		if proxyInfo.HTTP == nil {
			logrus.Warnf("no system http proxy found")
		} else {
			httpProxy := fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port)
			logrus.Infof("using http proxy: %q", httpProxy)
			finalEnv = append(finalEnv, httpProxy)
		}

		if proxyInfo.HTTPS == nil {
			logrus.Warnf("no system https proxy found")
		} else {
			httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
			logrus.Infof("using https proxy: %q", httpsProxy)
			finalEnv = append(finalEnv, httpsProxy)
		}
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		ShellCode:     static_resources.IgnitionScript(define.IgnitionVirtioFsTag, define.IgnitionFsMountDir),
		Workdir:       workdir,
		TargetBin:     bin,
		TargetBinArgs: args,
		Env:           finalEnv,
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

	defer func() {
		logrus.Infof("set memory: %dMB, cpus: %d", v.MemoryInMB, v.Cpus)
	}()

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

func (v *VMConfig) SetupIgnition() error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty, can not setup ignition virtio-fs")
	}

	guestAgentFilePath := filepath.Join(v.WorkspacePath, "ignition", "guest-agent")

	err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(guestAgentFilePath, static_resources.GuestAgentBytes, 0755)
	if err != nil {
		return err
	}

	v.Mounts = append(v.Mounts, define.Mount{
		ReadOnly: false,
		Type:     filesystem.VirtIOFs,
		Source:   filepath.Dir(guestAgentFilePath),
		Target:   define.IgnitionFsMountDir,
		Tag:      define.IgnitionVirtioFsTag,
		UUID:     define.IgnitionVirtioFsTag,
	})

	v.Ignition = define.Ignition{
		HostListenAddr: (&url.URL{
			Scheme: "unix",
			Path:   v.GetSocksPath("ignition-listen.sock"),
		}).String(),
		HostDir:  filepath.Join(v.WorkspacePath, "ignition"),
		GuestDir: define.IgnitionFsMountDir,
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
	logrus.Infof("log level: %s", l.String())

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
	err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsPath)
	if err != nil {
		return err
	}

	return v.WithRootfs(ctx, alpineRootfsPath)
}

func (v *VMConfig) WithRootfs(ctx context.Context, rootfsPath string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	rootfsPath, err := filepath.Abs(rootfsPath)
	if err != nil {
		return err
	}
	_, err = os.Stat(rootfsPath)
	if err != nil {
		logrus.Warnf("rootfs path %q does not exist, will try to extract it from builtin", rootfsPath)
		if err = static_resources.ExtractBuiltinRootfs(ctx, rootfsPath); err != nil {
			return err
		}
	}

	v.RootFS = rootfsPath

	// only one vm instance can access the rootfs at a time
	err = flock.New(filepath.Join(v.RootFS, ".lock")).Lock()
	if err != nil {
		return err
	}

	return nil
}

func (v *VMConfig) WithRunMode(runMode define.RunMode) {
	v.RunMode = runMode.String()
}

func (v *VMConfig) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "socks", name))
}

func (v *VMConfig) GetSSHPrivateKey() string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "ssh", "key"))
}

func (v *VMConfig) withPodmanCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetSocksPath("podman-listen.sock"),
	}

	v.PodmanInfo = define.PodmanInfo{
		PodmanAPIUnixSockLocalForward: unixAddr.String(),
		GuestPodmanAPIListenedIP:      define.GuestIP,
		GuestPodmanAPIListenedPort:    define.GuestPodmanAPIPort,
	}

	data, _ := json.Marshal(v.PodmanInfo)
	logrus.Infof("podman API configured with: %q", string(data))

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) withVMControlCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetSocksPath("vm-control.sock"),
	}

	v.VMCtlAddress = unixAddr.String()

	logrus.Infof("vm control API configured with: %q", v.VMCtlAddress)

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

	logrus.Infof("gvisor-tap-vsock configured with control endpoint: %q, network: %q", v.GvisorTapVsockEndpoint, v.GvisorTapVsockNetwork)

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) generateSSHCfg() error {
	keyPair, err := ssh.GenerateKeyPair(v.GetSSHPrivateKey(), ssh.DefaultKeyGenOptions())
	if err != nil {
		return err
	}

	v.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      keyPair.AuthorizedKey(),
		HostSSHPrivateKey:     string(keyPair.RawProtectedPrivateKey()),
		HostSSHPrivateKeyFile: v.GetSSHPrivateKey(),
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

	p, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return err
	}

	v.WorkspacePath = p

	if err = os.MkdirAll(v.WorkspacePath, 0755); err != nil {
		return err
	}

	// first we lock the workspace, only one vm instance can access the workspace at a time
	err = v.Lock()
	if err != nil {
		return err
	}

	err = v.withPodmanCfg()
	if err != nil {
		return err
	}

	err = v.withVMControlCfg()
	if err != nil {
		return err
	}

	err = v.withGvisorTapVsockCfg()
	if err != nil {
		return err
	}

	if err = v.generateSSHCfg(); err != nil {
		return err
	}

	return nil
}

func NewVMConfig(mode define.RunMode) *VMConfig {
	vmc := &VMConfig{
		RunMode: mode.String(),
	}

	return vmc
}
