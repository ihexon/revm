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
	logrus.Infof("ssh keypair write to: %q", vmc.SSHInfo.HostSSHKeyPairFile)

	// Fill the ssh keypair into vmc.SSHInfo using the keypair in memory
	vmc.SSHInfo.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	vmc.SSHInfo.HostSSHPublicKey = keyPair.AuthorizedKey()

	sshPort, err := network.GetAvailablePort("tcp4")
	if err != nil {
		return fmt.Errorf("failed to get available port for ssh: %w", err)
	}

	vmc.SSHInfo.GuestAddr = define.DefaultGuestSSHListenAddr
	vmc.SSHInfo.Port = sshPort
	vmc.SSHInfo.User = define.DefaultGuestUser
	logrus.Infof("guest ssh running on %s@%s:%d", vmc.SSHInfo.User, vmc.SSHInfo.GuestAddr, vmc.SSHInfo.Port)

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
	if len(disks) == 0 || disks == nil {
		logrus.Debugf("the data disk information provided by the user is empty, so the data disk will not be mounted to the guest")
		return nil
	}

	for _, disk := range disks {
		diskAbsPath, err := filepath.Abs(disk)
		if err != nil {
			return fmt.Errorf("failed to get abs path: %w", err)
		}

		rawDisk := filesystem.NewDisk(diskAbsPath)

		if !system.IsPathExist(diskAbsPath) {
			if err := rawDisk.CreateExt4DiskAndFormat(ctx); err != nil {
				return fmt.Errorf("failed to create ext4 disk: %w", err)
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
	if cpus <= 0 {
		logrus.Warnf("1 cpu cores is the minimum value and has been enforced")
		cpus = 1
	}

	if memory <= 512 {
		logrus.Warnf("512MB of memory is the minimum value and has been enforced")
		memory = 512
	}

	vmc.MemoryInMB = memory
	vmc.Cpus = cpus
}

func (vmc *VMConfig) WithShareUserHomeDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("can not get user home directory: %w", err)
	}

	return vmc.WithUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (vmc *VMConfig) WithUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		logrus.Debugf("the directory mount information provided by the user is empty, so the host directory will not be mounted to the guest")
		return nil
	}

	if vmc.Mounts == nil {
		return fmt.Errorf("vmc.Mount is nil")
	}

	var hostDirs []string
	for _, dir := range dirs {
		p, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		hostDirs = append(hostDirs, p)
	}

	vmc.Mounts = append(vmc.Mounts, filesystem.CmdLineMountToMounts(hostDirs)...)
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

	if _, err = vmc.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock for rootfs: %v", err)
	}

	return nil
}

func (vmc *VMConfig) WithBuiltInRootfs() error {
	path, err := system.GetBuiltinRootfsPath()
	if err != nil {
		return fmt.Errorf("failed to get builtin rootfs path: %w", err)
	}

	return vmc.WithUserProvidedRootFS(path)
}

func (vmc *VMConfig) WithContainerDataDisk(ctx context.Context, disk string) error {
	if disk == "" {
		return fmt.Errorf("container storage disk is empty")
	}

	if vmc.DataDisk == nil {
		return fmt.Errorf("vmc.DataDisk is nil")
	}

	containerDisk := filesystem.NewDisk(disk)
	containerDisk.IsContainerStorage = true

	if !system.IsPathExist(containerDisk.Path) {
		if err := containerDisk.CreateExt4DiskAndFormat(ctx); err != nil {
			return fmt.Errorf("failed to create container ext4 disk: %w", err)
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
func (vmc *VMConfig) WithRESTAPIAddress(path string) error {
	if path == "" {
		return fmt.Errorf("restapi listening path is empty")
	}

	listenAPIPath, err := filepath.Abs(path)
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
		GVProxyChan:     make(chan struct{}, 1),
		IgnServerChan:   make(chan struct{}, 1),
		PodmanReadyChan: make(chan struct{}, 1),
		SSHDReadyChan:   make(chan struct{}, 1),
	}

	vmc.DataDisk = []define.DataDisk{}
	vmc.Mounts = []define.Mount{}

	return vmc
}
