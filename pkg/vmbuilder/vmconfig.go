//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/event"
	"linuxvm/pkg/filesystem"
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
	VM define.Machine
)

func (v *VM) Lock() error {
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

func (v *VM) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
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

func (v *VM) GetContainerStorageDiskPath() string {
	return NewPathManager(v.WorkspacePath).GetContainerStorageDiskPath()
}

func (v *VM) ConfigureContainerRAWDisk(ctx context.Context) error {
	rawDiskFilePath := v.GetContainerStorageDiskPath()
	if _, err := os.Stat(rawDiskFilePath); err != nil {
		if err = v.generateRAWDisk(ctx, rawDiskFilePath, define.ContainerDiskUUID); err != nil {
			return err
		}
	}

	return v.addRAWDiskToBlkList(ctx, rawDiskFilePath)
}

func (v *VM) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string) error {
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

func (v *VM) WithMountUserHome(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.withUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VM) withUserProvidedStorageRAWDisk(ctx context.Context, rawDiskS []string) error {
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

func (v *VM) SetupCmdLine(workdir, bin string, args, envs []string) error {
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

func (v *VM) WithResources(memoryInMB uint64, cpus int8) error {
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

func (v *VM) withUserProvidedMounts(dirs []string) error {
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

func (v *VM) ConfigureGuestAgent() error {
	pathMgr := NewPathManager(v.WorkspacePath)
	configurator := NewGuestAgentConfigurator(pathMgr)
	return configurator.Configure(context.Background(), (*define.Machine)(v))
}

func (v *VM) SetupLogLevel(level string) error {
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

	v.LogFilePath = filepath.Join(v.WorkspacePath, "logs", "vm.log")

	if err := os.MkdirAll(filepath.Dir(v.LogFilePath), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	if info, err := os.Stat(v.LogFilePath); err == nil && info.Size() > int64(filesystem.MiB(10).ToBytes()) {
		_ = os.Truncate(v.LogFilePath, 0)
	}

	f, err := os.OpenFile(v.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))

	return nil
}

func (v *VM) getRootfsDir() string {
	return NewPathManager(v.WorkspacePath).GetRootfsDir()
}

func (v *VM) WithBuiltInAlpineRootfs(ctx context.Context) error {
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
	event.Emit(event.RootfsExtractedReady)

	return v.WithUserProvidedRootfs(ctx, alpineRootfsDir)
}

func (v *VM) WithUserProvidedRootfs(ctx context.Context, rootfsPath string) error {
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

func (v *VM) GetSSHPrivateKeyFile() string {
	return NewPathManager(v.WorkspacePath).GetSSHPrivateKeyFile()
}

func (v *VM) configureVirtualMachineControlAPI() error {
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

func (v *VM) ResetOrReuseContainerRAWDisk(ctx context.Context, containerDiskVersionXATTR string) error {
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

func (v *VM) ConfigureVirtualNetwork(ctx context.Context, mode define.VNetMode) error {
	v.VirtualNetworkMode = mode

	// Use strategy pattern for network configuration
	strategy := GetNetworkStrategy(mode)
	if strategy == nil {
		return fmt.Errorf("invalid virtual network mode: %s", mode)
	}

	pathMgr := NewPathManager(v.WorkspacePath)
	return strategy.Configure(ctx, (*define.Machine)(v), pathMgr)
}

func (v *VM) generateSSHCfg() error {
	pathMgr := NewPathManager(v.WorkspacePath)
	configurator := NewSSHConfigurator(pathMgr)
	return configurator.Configure(context.Background(), (*define.Machine)(v))
}

func (v *VM) SetupWorkspace(workspacePath string) error {
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

	if err = v.Lock(); err != nil {
		return err
	}

	return v.generateSSHCfg()
}

func NewVirtualMachine(mode define.RunMode) *VM {
	vmc := &VM{
		RunMode:       mode.String(),
		XATTRSRawDisk: map[string]string{},
		StopCh:        make(chan struct{}),
		Readiness:     define.NewReadiness(),
	}

	return vmc
}

func (v *VM) needsDiskRegeneration(ctx context.Context) (bool, error) {
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

func (v *VM) WithRAWDiskVersionXATTR(value string) *VM {
	v.XATTRSRawDisk = map[string]string{
		define.XATTRRawDiskVersionKey: value,
	}
	return v
}

func (v *VM) GetPodmanListenAddr() string {
	return NewPathManager(v.WorkspacePath).GetPodmanListenAddr()
}

func (v *VM) GetVNetListenAddr() string {
	return NewPathManager(v.WorkspacePath).GetVNetListenAddr()
}

func (v *VM) GetGVPCtlAddr() string {
	return NewPathManager(v.WorkspacePath).GetGVPCtlAddr()
}

func (v *VM) GetVMCtlAddr() string {
	return NewPathManager(v.WorkspacePath).GetVMCtlAddr()
}

func (v *VM) GetIgnAddr() string {
	return NewPathManager(v.WorkspacePath).GetIgnAddr()
}

// PathManager handles all workspace-related path calculations.
type PathManager struct {
	workspacePath string
}

// NewPathManager creates a new PathManager for the given workspace path.
func NewPathManager(workspacePath string) *PathManager {
	return &PathManager{workspacePath: workspacePath}
}

func (p *PathManager) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(p.workspacePath, "socks", name))
}

func (p *PathManager) GetPodmanListenAddr() string {
	return p.GetSocksPath("podman-api.sock")
}

func (p *PathManager) GetVNetListenAddr() string {
	return p.GetSocksPath("vnet.sock")
}

func (p *PathManager) GetGVPCtlAddr() string {
	return p.GetSocksPath("gvpctl.sock")
}

func (p *PathManager) GetVMCtlAddr() string {
	return p.GetSocksPath("vmctl.sock")
}

func (p *PathManager) GetIgnAddr() string {
	return p.GetSocksPath("ign.sock")
}

func (p *PathManager) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(p.workspacePath, "ssh", "key"))
}

func (p *PathManager) GetLogsDir() string {
	return filepath.Join(p.workspacePath, "logs")
}

func (p *PathManager) GetRootfsDir() string {
	return filepath.Join(p.workspacePath, "rootfs")
}

func (p *PathManager) GetContainerStorageDiskPath() string {
	return filepath.Join(p.workspacePath, "raw-disk", "container-storage.ext4")
}
