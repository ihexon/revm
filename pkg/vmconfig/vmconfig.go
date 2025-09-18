//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"net/url"
	"os"
	"strings"

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

func (vmc *VMConfig) WithSystemProxy() error {
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

func (vmc *VMConfig) WithDataDisk(ctx context.Context, disks []string) error {
	var createError error
	cleanup := system.CleanUp()
	defer cleanup.CleanIfErr(&createError)

	for _, disk := range disks {
		diskAbsPath, err := filepath.Abs(disk)
		if err != nil {
			return fmt.Errorf("failed to get abs path: %w", err)
		}

		rawDisk := filesystem.NewDisk(diskAbsPath)

		if !system.IsPathExist(diskAbsPath) {
			rawDisk.SetFileSystemType(define.Ext4)
			rawDisk.SetUUID(uuid.New().String())
			rawDisk.SetSizeInGB(define.DefaultCreateDiskSizeInGB)

			cleanup.Add(func() error {
				return os.Remove(rawDisk.Path)
			})

			if createError = rawDisk.Create(); createError != nil {
				return createError
			}

			if createError = rawDisk.Format(ctx); createError != nil {
				return createError
			}
		}

		if err := rawDisk.Inspect(ctx); err != nil {
			return fmt.Errorf("failed to inspect raw disk %q, %w", diskAbsPath, err)
		}

		if err := rawDisk.FsCheck(ctx); err != nil {
			return fmt.Errorf("failed to run fsck to raw disk %q, %w", diskAbsPath, err)
		}

		vmc.DataDisk = append(vmc.DataDisk, define.DataDisk(rawDisk))
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

func (vmc *VMConfig) SetGuestBootstrapRunArgs() {
	vmc.Cmdline.Bootstrap = system.GetGuestLinuxUtilsBinPath(define.BoostrapFileName)
	vmc.Cmdline.Env = []string{define.DefaultPATHInBootstrap}

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		vmc.Cmdline.BootstrapArgs = append(vmc.Cmdline.BootstrapArgs, "--verbose")
	}
}

func (vmc *VMConfig) WithUserProvidedCmdline(bin string, args, envs []string) error {
	if strings.TrimSpace(bin) == "" {
		return fmt.Errorf("target binary is empty")
	}

	if err := validateArgs(args); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}

	vmc.Cmdline.Workspace = define.DefaultWorkDir
	vmc.Cmdline.TargetBin = bin
	vmc.Cmdline.TargetBinArgs = args
	vmc.Cmdline.Env = append(vmc.Cmdline.Env, envs...)

	return nil
}

func (vmc *VMConfig) WithResources(memory uint64, cpus int8) {
	vmc.MemoryInMB = memory
	vmc.Cpus = cpus
}

func (vmc *VMConfig) WithUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return fmt.Errorf("mount dirs is empty, please check your input")
	}

	if vmc.Mounts == nil {
		return fmt.Errorf("vmc.Mount is nil")
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

func (vmc *VMConfig) WithContainerDataDisk(ctx context.Context, disk string) error {
	if disk == "" {
		return fmt.Errorf("container storage disk is empty")
	}

	if vmc.DataDisk == nil {
		return fmt.Errorf("vmc.DataDisk is nil")
	}
	var createError error
	cleanup := system.CleanUp()
	defer cleanup.CleanIfErr(&createError)

	containerDisk := filesystem.NewDisk(disk)
	containerDisk.IsContainerStorage = true

	if !system.IsPathExist(containerDisk.Path) {
		containerDisk.SetFileSystemType(define.Ext4)
		containerDisk.SetUUID(uuid.New().String())
		containerDisk.SetSizeInGB(define.DefaultCreateDiskSizeInGB)

		cleanup.Add(func() error {
			return os.Remove(containerDisk.Path)
		})

		if createError = containerDisk.Create(); createError != nil {
			return createError
		}

		if createError = containerDisk.Format(ctx); createError != nil {
			return createError
		}
	}

	if err := containerDisk.Inspect(ctx); err != nil {
		return fmt.Errorf("failed to inspect container disk %q, %w", disk, err)
	}

	if err := containerDisk.FsCheck(ctx); err != nil {
		return fmt.Errorf("failed to run fscheck container disk %q, %w", disk, err)
	}

	logrus.Infof("in docker mode, container storage disk will be %q", disk)
	vmc.DataDisk = append(vmc.DataDisk, define.DataDisk(containerDisk))

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

	vmc.DataDisk = []define.DataDisk{}
	vmc.Mounts = []define.Mount{}

	return vmc
}
