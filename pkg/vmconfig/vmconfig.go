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
	sshv2 "linuxvm/pkg/ssh_v2"
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

	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("get lock failed: %w", err)
	}

	if !ifLocked {
		return fmt.Errorf("workspace %q is locked by another instance", fileLock.Path())
	}

	return nil
}

func (v *VMConfig) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
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

func (v *VMConfig) GetContainerStorageDiskPath() string {
	return filepath.Join(v.WorkspacePath, "raw-disk", "container-storage.ext4")
}

func (v *VMConfig) ConfigurePodmanUsingSystemProxy() error {
	var envs []string

	logrus.Warnf("your system proxy must support CONNECT method")
	proxyInfo, err := network.GetAndNormalizeSystemProxy()
	if err != nil {
		return fmt.Errorf("failed to get and normalize system proxy: %w", err)
	}

	if proxyInfo.HTTP != nil {
		envs = append(envs, fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port))
	}

	if proxyInfo.HTTPS != nil {
		envs = append(envs, fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port))
	}

	v.PodmanInfo.Envs = append(v.PodmanInfo.Envs, envs...)

	return nil
}

func (v *VMConfig) AttachOrGenerateContainerStorageRawDisk(ctx context.Context) error {
	rawDiskFilePath := v.GetContainerStorageDiskPath()
	if _, err := os.Stat(rawDiskFilePath); err != nil {
		if err = v.generateRAWDisk(ctx, rawDiskFilePath, define.ContainerDiskUUID); err != nil {
			return err
		}
	}

	return v.addRAWDiskToBlkList(ctx, rawDiskFilePath)
}

func (v *VMConfig) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string) error {
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

func (v *VMConfig) BindUserHomeDir(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.WithMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VMConfig) WithUserProvidedStorageRAWDisk(ctx context.Context, rawDiskS []string) error {
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
			envs = append(envs, fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port))
		}

		if proxyInfo.HTTPS != nil {
			envs = append(envs, fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port))
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
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomainInGVPNet))
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
	logrus.Infof("ignition server port will listen in: %d", port)

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   v.GetIgnAddr(),
	}

	if err = os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	_ = os.Remove(unixUSL.Path)

	v.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenSockAddr: unixUSL.String(),
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
		if info, err := os.Stat(logFile); err == nil && info.Size() > maxSizeBytes {
			_ = os.Truncate(logFile, 0)
		}

		if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logrus.SetOutput(f)
			logger.LogFd = f
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

func (v *VMConfig) getSocksPath(name string) string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "socks", name))
}

func (v *VMConfig) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(v.WorkspacePath, "ssh", "key"))
}

func (v *VMConfig) withPodmanCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetPodmanListenAddr(),
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
		Path:   v.GetVMCtlAddr(),
	}

	v.VMCtlAddress = unixAddr.String()

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) withGvisorTapVsockCfg() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetGVPCtlAddr(),
	}

	v.GVPCtlAddr = unixAddr.String()

	v.GVPVNetAddr = fmt.Sprintf("unixgram://%s", v.GetVNetListenAddr())

	_ = os.Remove(v.GetGVPCtlAddr())
	_ = os.Remove(v.GetVNetListenAddr())

	v.TSI = false // disable libkrun TSI networking backend when using gvisor-tap-vsock virtualNet

	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}

func (v *VMConfig) generateSSHCfg() error {
	keyPath := v.GetSSHPrivateKeyFile()
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
		HostSSHPublicKey:      string(publicKey),
		HostSSHPrivateKey:     string(privateKey),
		HostSSHPrivateKeyFile: keyPath,
	}

	return nil
}

func (v *VMConfig) SetupWorkspace(workspacePath string) error {
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

	if workspacePath == homeDir {
		return fmt.Errorf("workspace can not be the home dir")
	}

	v.WorkspacePath = workspacePath

	if err = os.MkdirAll(v.WorkspacePath, 0755); err != nil {
		return err
	}

	if err = v.Lock(); err != nil {
		return err
	}

	if v.RunMode == define.ContainerMode.String() {
		if err = v.withPodmanCfg(); err != nil {
			return err
		}
	}

	if err = v.withVMControlCfg(); err != nil {
		return err
	}

	if err = v.withGvisorTapVsockCfg(); err != nil {
		return err
	}

	return v.generateSSHCfg()
}

func NewVMConfig(mode define.RunMode) *VMConfig {
	vmc := &VMConfig{
		RunMode:       mode.String(),
		XATTRSRawDisk: map[string]string{},
		StopCh:        make(chan struct{}),
	}

	return vmc
}

func (v *VMConfig) NeedsDiskRegeneration(ctx context.Context) (bool, error) {
	xattrKey := define.XATTRRawDiskVersionKey
	xattrProcesser := filesystem.NewXATTRManager()

	value1, _ := xattrProcesser.GetXATTR(ctx, v.GetContainerStorageDiskPath(), xattrKey)
	value2 := v.XATTRSRawDisk[xattrKey]
	if value2 == "" {
		return false, fmt.Errorf("vmc XATTRSRawDisk not set")
	}

	if value1 != value2 {
		return true, nil
	}

	return false, nil
}

func (v *VMConfig) WithRAWDiskVersionXATTR(value string) *VMConfig {
	v.XATTRSRawDisk = map[string]string{
		define.XATTRRawDiskVersionKey: value,
	}
	return v
}

func (v *VMConfig) GetPodmanListenAddr() string {
	return v.getSocksPath("podman-api.sock")
}

func (v *VMConfig) GetVNetListenAddr() string {
	return v.getSocksPath("vnet.sock")
}

func (v *VMConfig) GetGVPCtlAddr() string {
	return v.getSocksPath("gvpctl.sock")
}

func (v *VMConfig) GetVMCtlAddr() string {
	return v.getSocksPath("vmctl.sock")
}

func (v *VMConfig) GetIgnAddr() string {
	return v.getSocksPath("ign.sock")
}

func (v *VMConfig) WithNetworkTSI() error {
	// clean gvisor-tap-vsock sockets
	_ = os.Remove(v.GetGVPCtlAddr())
	_ = os.Remove(v.GetVNetListenAddr())
	v.GVPVNetAddr = ""
	v.GVPCtlAddr = ""

	v.TSI = true

	return nil
}

func LoadVMCFromFile(file string) (*VMConfig, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	vmc := &VMConfig{}

	if err = json.NewDecoder(f).Decode(vmc); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %w", file, err)
	}
	return vmc, nil
}
