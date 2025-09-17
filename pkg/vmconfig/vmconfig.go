//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"net/url"
	"os"

	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// VMConfig Static virtual machine configuration.

type (
	VMConfig define.VMConfig
)

// ParseDiskInfo Parse data disk information provided by user, get the filesystem type and uuid, save them to vmc.DataDisk
// and vmc.ContainerStorage.
func (vmc *VMConfig) ParseDiskInfo(ctx context.Context) error {
	for _, disk := range vmc.DataDisk {
		info, err := filesystem.GetBlockInfo(ctx, disk.Path)
		if err != nil {
			return fmt.Errorf("failed to get block %q info: %w", disk.Path, err)
		}

		disk.UUID = info.UUID
		disk.FileSystemType = info.FilesystemType
		disk.Path = info.AbsPath
		disk.MountPoint = filepath.Join(define.DefaultDataDiskMountDirPrefix, info.AbsPath)
		if disk.IsContainerStorage {
			disk.MountPoint = define.ContainerStorageMountPoint
		}

		if disk.IsContainerStorage {
			err := filesystem.Fscheck(ctx, disk.Path)
			if err != nil {
				return fmt.Errorf("failed to check container storage %q: %w", disk.Path, err)
			}
		}

		// Print disk info for debug
		jsonData, err := json.MarshalIndent(disk, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal disk info: %w", err)
		}
		logrus.Debugf("disk %q info: %s", disk.Path, jsonData)

		logrus.Infof("the disk: %q will be mount into %q", disk.Path, disk.MountPoint)
	}

	return nil
}

func (vmc *VMConfig) CreateRawDiskWhenNeeded(ctx context.Context) error {
	for _, disk := range vmc.DataDisk {
		if system.IsPathExist(disk.Path) {
			logrus.Debugf("disk %q already exist, mark this disk as reuseable disk", disk.Path)
			disk.ReUse = true
		} else {
			logrus.Debugf("disk %q not exist, mark this disk not reusable", disk.Path)
			disk.ReUse = false
		}

		if !disk.ReUse {
			if err := filesystem.CreateDiskAndFormatExt4(ctx, disk.Path, uuid.NewString(), true); err != nil {
				return fmt.Errorf("failed to create raw disk %q: %w", disk.Path, err)
			}
		}
	}

	return vmc.ParseDiskInfo(ctx)
}

func (vmc *VMConfig) TryGetSystemProxyAndSetToCmdline() error {
	proxyInfo, err := network.GetAndNormalizeSystemProxy()
	if err != nil {
		return fmt.Errorf("failed to get and normalize system proxy: %w", err)
	}

	if proxyInfo.HTTP == nil {
		logrus.Warnf("no system http proxy found")
	} else {
		httpProxy := fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port)
		logrus.Infof("using http proxy: %q", httpProxy)
		vmc.Cmdline.Env = append(vmc.Cmdline.Env, httpProxy)
	}

	if proxyInfo.HTTPS == nil {
		logrus.Warnf("no system https proxy found")
	} else {
		httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
		logrus.Infof("using https proxy: %q", httpsProxy)
		vmc.Cmdline.Env = append(vmc.Cmdline.Env, httpsProxy)
	}

	return nil
}

// GenerateSSHInfo Generate SSH info for the VM, notice the ssh keypair will be written when guest rootfs actually running.
func (vmc *VMConfig) GenerateSSHInfo() error {
	keyPair, err := ssh.GenerateHostSSHKeyPair(vmc.SSHInfo.HostSSHKeyPairFile)
	if err != nil {
		return fmt.Errorf("failed to generate host ssh keypair for host: %w", err)
	}

	// Fill the ssh keypair into vmc.SSHInfo using the keypair in memory
	vmc.SSHInfo.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	vmc.SSHInfo.HostSSHPublicKey = keyPair.AuthorizedKey()

	sshPort, err := network.GetAvailablePort("tcp4")
	if err != nil {
		return fmt.Errorf("failed to get available port: %w", err)
	}

	vmc.SSHInfo.GuestAddr = define.DefaultGuestSSHListenAddr
	vmc.SSHInfo.Port = sshPort
	vmc.SSHInfo.User = define.DefaultGuestUser
	logrus.Debugf("guest ssh listen addr: %q, port %d, user %q", vmc.SSHInfo.GuestAddr, vmc.SSHInfo.Port, vmc.SSHInfo.User)

	return nil
}

func (vmc *VMConfig) Lock() (*flock.Flock, error) {
	if vmc.RootFS == "" {
		return nil, fmt.Errorf("root file system is not set")
	}

	fileLock := flock.New(filepath.Join(vmc.RootFS, define.LockFile))

	logrus.Debugf("try to lock file: %q", fileLock.Path())
	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}

	if !ifLocked {
		return nil, fmt.Errorf("file %q is locked by another vm instance", fileLock.Path())
	}

	return fileLock, nil
}

func (vmc *VMConfig) WaitGVProxyReady(ctx context.Context) {
	logrus.Debugf("waiting gvproxy ready")
	waitGVProxyReady(ctx, vmc)
	logrus.Debugf("gvproxy ready")
}

func (vmc *VMConfig) WaitIgnServerReady(ctx context.Context) {
	logrus.Debugf("waiting Ignition server ready")
	waitIgnServerReady(ctx, vmc)
	logrus.Debugf("Ignition server ready")
}

func waitGVProxyReady(ctx context.Context, vmc *VMConfig) {
	select {
	case <-ctx.Done():
		return
	case <-vmc.Stage.GVProxyChan:
		return
	}
}

func waitIgnServerReady(ctx context.Context, vmc *VMConfig) {
	select {
	case <-ctx.Done():
		return
	case <-vmc.Stage.IgnServerChan:
		return
	}
}

func (vmc *VMConfig) WithUserProvidedDataDisk(disks []string) error {
	if len(disks) == 0 || disks == nil {
		return fmt.Errorf("data disk list is empty, please check your input")
	}

	var dataDisks []*define.DataDisk
	for _, disk := range disks {
		diskPath, err := filepath.Abs(disk)
		if err != nil {
			return fmt.Errorf("failed to get abs path: %w", err)
		}

		dataDisks = append(dataDisks, &define.DataDisk{
			Path: diskPath,
		})
	}
	vmc.DataDisk = dataDisks

	return nil
}

func (vmc *VMConfig) WithUserProvidedCmdline(bin string, args, envs []string) {
	vmc.Cmdline = define.Cmdline{
		Bootstrap:     system.GetGuestLinuxUtilsBinPath(define.BoostrapFileName),
		BootstrapArgs: []string{},
		Workspace:     define.DefaultWorkDir,
		TargetBin:     bin,
		TargetBinArgs: args,
		Env:           append(envs, define.DefaultPATHInBootstrap),
	}
}

func (vmc *VMConfig) WithResources(memory uint64, cpus int8) {
	vmc.MemoryInMB = memory
	vmc.Cpus = cpus
}

func (vmc *VMConfig) WithUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return fmt.Errorf("mount dirs is empty, please check your input")
	}

	var absDirs []string
	for _, dir := range dirs {
		p, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("failed to get abs path: %w", err)
		}
		absDirs = append(absDirs, p)
	}

	vmc.Mounts = append(vmc.Mounts, filesystem.CmdLineMountToMounts(absDirs)...)
	return nil
}

func (vmc *VMConfig) WithUserProvidedRootFS(rootfsPath string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path is empty")
	}
	rootfsPath, err := filepath.Abs(rootfsPath)
	if err != nil {
		return err
	}

	vmc.RootFS = rootfsPath
	return nil
}

func (vmc *VMConfig) WithBuiltInRootfs() error {
	path, err := system.GetBuiltinRootfsPath()
	if err != nil {
		return fmt.Errorf("failed to get builtin rootfs path: %w", err)
	}
	vmc.RootFS = path

	return nil
}

func (vmc *VMConfig) WithContainerDataDisk(disk string) error {
	if disk == "" {
		return fmt.Errorf("container storage disk is empty")
	}

	disk, err := filepath.Abs(disk)
	if err != nil {
		return fmt.Errorf("failed to get abs path: %w", err)
	}

	logrus.Infof("in docker mode, container storage disk will be %q", disk)
	vmc.DataDisk = append(vmc.DataDisk, &define.DataDisk{
		IsContainerStorage: true,
		Path:               disk,
	})

	return nil
}

func (vmc *VMConfig) WithPodmanListenAPIInHost(listenAPIPath string) error {
	if listenAPIPath == "" {
		return fmt.Errorf("listen api path is empty")
	}

	listenAPIPath, err := filepath.Abs(listenAPIPath)
	if err != nil {
		return err
	}

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   listenAPIPath,
	}

	vmc.PodmanInfo = define.PodmanInfo{
		UnixSocksAddr: unixAddr.String(),
	}
	return nil
}

// WithRESTAPIAddress set the REST API address for the VM. only support unix socket
func (vmc *VMConfig) WithRESTAPIAddress(listenAPIPath string) error {
	if listenAPIPath == "" {
		return fmt.Errorf("listen api path is empty")
	}

	listenAPIPath, err := filepath.Abs(listenAPIPath)
	if err != nil {
		return err
	}

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   listenAPIPath,
	}

	vmc.RestAPIAddress = unixAddr.String()
	return nil
}

func NewVMConfig() *VMConfig {
	vmc := &VMConfig{}

	prefix := filepath.Join(os.TempDir(), system.GenerateRandomID())
	logrus.Debugf("runtime temp directory: %q", prefix)

	vmc.GVproxyEndpoint = fmt.Sprintf("unix://%s/%s", prefix, define.GvProxyControlEndPoint)
	vmc.NetworkStackBackend = fmt.Sprintf("unixgram://%s/%s", prefix, define.GvProxyNetworkEndpoint)
	vmc.SSHInfo = define.SSHInfo{
		HostSSHKeyPairFile: filepath.Join(prefix, define.SSHKeyPair),
	}
	vmc.IgnProvisionerAddr = fmt.Sprintf("unix://%s/%s", prefix, define.IgnServerSocketName)
	vmc.Stage = define.Stage{
		GVProxyChan:   make(chan struct{}, 1),
		IgnServerChan: make(chan struct{}, 1),
	}

	return vmc
}
