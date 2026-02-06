//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/networkmode"
	"linuxvm/pkg/static_resources"
	"linuxvm/pkg/vmconfig/internal"
	vnetwork "linuxvm/pkg/vmconfig/network"
	"linuxvm/pkg/vmconfig/services"
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
	return internal.NewPathManager(v.WorkspacePath).GetContainerStorageDiskPath()
}

func (v *VMConfig) ConfigureGuestPodman(ifUsingSystemProxy bool) error {
	pathMgr := internal.NewPathManager(v.WorkspacePath)
	configurator := services.NewPodmanConfigurator(pathMgr)
	return configurator.Configure(context.Background(), (*define.VMConfig)(v), ifUsingSystemProxy)
}

func (v *VMConfig) ConfigureContainerRAWDisk(ctx context.Context) error {
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

func (v *VMConfig) WithMountUserHome(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.withUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VMConfig) withUserProvidedStorageRAWDisk(ctx context.Context, rawDiskS []string) error {
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

func (v *VMConfig) withUserProvidedMounts(dirs []string) error {
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

func (v *VMConfig) ConfigureGuestAgent() error {
	pathMgr := internal.NewPathManager(v.WorkspacePath)
	configurator := services.NewGuestAgentConfigurator(pathMgr)
	return configurator.Configure(context.Background(), (*define.VMConfig)(v))
}

func (v *VMConfig) SetupLogLevel(level string) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	logFile := v.GetVMMRunLogsFile()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	if info, err := os.Stat(logFile); err == nil && info.Size() > int64(filesystem.MiB(10).ToBytes()) {
		_ = os.Truncate(logFile, 0)
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))

	return nil
}

func (v *VMConfig) getRootfsDir() string {
	return internal.NewPathManager(v.WorkspacePath).GetRootfsDir()
}

func (v *VMConfig) WithBuiltInAlpineRootfs(ctx context.Context) error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	alpineRootfsDir := v.getRootfsDir()
	if err := os.MkdirAll(alpineRootfsDir, 0755); err != nil {
		return err
	}

	if err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsDir); err != nil {
		return err
	}

	return v.WithUserProvidedRootfs(ctx, alpineRootfsDir)
}

func (v *VMConfig) WithUserProvidedRootfs(ctx context.Context, rootfsPath string) error {
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

func (v *VMConfig) GetSSHPrivateKeyFile() string {
	return internal.NewPathManager(v.WorkspacePath).GetSSHPrivateKeyFile()
}

func (v *VMConfig) configureVirtualMachineControlAPI() error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   v.GetVMCtlAddr(),
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

func (v *VMConfig) ResetOrReuseContainerRAWDisk(ctx context.Context, containerDiskVersionXATTR string) error {
	resetBool, err := v.WithRAWDiskVersionXATTR(containerDiskVersionXATTR).needsDiskRegeneration(ctx)
	if err != nil {
		return fmt.Errorf("failed to check RAW disk needs to regenerate: %w", err)
	}

	if resetBool {
		if err := os.Remove(v.GetContainerStorageDiskPath()); err != nil && !os.IsNotExist(err) {
			return err
		}

		if err := v.ConfigureContainerRAWDisk(ctx); err != nil {
			return fmt.Errorf("failed to attach container storage raw disk: %w", err)
		}
	}

	return nil
}

func (v *VMConfig) ConfigureVirtualNetwork(ctx context.Context, mode define.VNetMode) error {
	v.VirtualNetworkMode = mode.String()

	// Use strategy pattern for network configuration
	strategy := vnetwork.GetNetworkStrategy(mode)
	if strategy == nil {
		return fmt.Errorf("invalid virtual network mode: %s", mode)
	}

	pathMgr := internal.NewPathManager(v.WorkspacePath)
	return strategy.Configure(ctx, (*define.VMConfig)(v), pathMgr)
}

func (v *VMConfig) generateSSHCfg() error {
	pathMgr := internal.NewPathManager(v.WorkspacePath)
	configurator := services.NewSSHConfigurator(pathMgr)
	return configurator.Configure(context.Background(), (*define.VMConfig)(v))
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

func (v *VMConfig) GetVMMRunLogsFile() string {
	return filepath.Join(internal.NewPathManager(v.WorkspacePath).GetLogsDir(), "vm.log")
}

func (v *VMConfig) needsDiskRegeneration(ctx context.Context) (bool, error) {
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
	return internal.NewPathManager(v.WorkspacePath).GetPodmanListenAddr()
}

func (v *VMConfig) GetVNetListenAddr() string {
	return internal.NewPathManager(v.WorkspacePath).GetVNetListenAddr()
}

func (v *VMConfig) GetGVPCtlAddr() string {
	return internal.NewPathManager(v.WorkspacePath).GetGVPCtlAddr()
}

func (v *VMConfig) GetVMCtlAddr() string {
	return internal.NewPathManager(v.WorkspacePath).GetVMCtlAddr()
}

func (v *VMConfig) GetIgnAddr() string {
	return internal.NewPathManager(v.WorkspacePath).GetIgnAddr()
}

// GetNetworkMode returns the network mode as a first-class Mode object.
// This provides a clean abstraction over network mode differences.
func (v *VMConfig) GetNetworkMode() networkmode.Mode {
	return networkmode.FromString(v.VirtualNetworkMode)
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
