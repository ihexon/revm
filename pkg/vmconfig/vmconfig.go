//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	"fmt"

	"net/url"
	"os"
	"strings"

	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/path"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

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
	logrus.Infof("ssh keypair write to: %q", vmc.SSHInfo.HostSSHKeyPairFile)
	keyPair, err := ssh.GenerateHostSSHKeyPair(vmc.SSHInfo.HostSSHKeyPairFile)
	if err != nil {
		return fmt.Errorf("failed to generate host ssh keypair for host: %w", err)
	}

	// Fill the ssh keypair into vmc.SSHInfo using the keypair in memory
	vmc.SSHInfo.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	vmc.SSHInfo.HostSSHPublicKey = keyPair.AuthorizedKey()

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
	waitGVProxyReady(ctx, vmc)
}

func (vmc *VMConfig) WaitIgnServerReady(ctx context.Context) {
	waitIgnServerReady(ctx, vmc)
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

func (vmc *VMConfig) WithBlkDisk(ctx context.Context, blocks []string, isContainerDisk bool) error {
	diskMgr := disk.NewBlkManager(
		vmc.ExternalTools.DarwinTools.MkfsExt4,
		vmc.ExternalTools.DarwinTools.Blkid,
		vmc.ExternalTools.DarwinTools.FsckExt4,
	)

	for _, blk := range blocks {
		absPath, err := filepath.Abs(blk)
		if err != nil {
			return err
		}

		if !system.IsPathExist(absPath) {
			logrus.Infof("create ext4 block %q with size %d", absPath, define.DefaultCreateDiskSizeInGB)

			if err := diskMgr.Create(ctx, absPath, define.DefaultCreateDiskSizeInGB*1024); err != nil {
				return fmt.Errorf("failed to create ext4 disk: %w", err)
			}
		}

		info, err := diskMgr.Inspect(ctx, absPath)
		if err != nil {
			return fmt.Errorf("failed to inspect raw disk %q, %w", absPath, err)
		}

		if err := diskMgr.FsCheck(ctx, absPath); err != nil {
			return fmt.Errorf("filesystem integrity check %q error %w", absPath, err)
		}

		blkDev := define.BlkDev{
			UUID:               info.UUID,
			FsType:             info.FsType,
			IsContainerStorage: isContainerDisk,
			Path:               absPath,
			MountTo:            filepath.Join("/mnt", absPath),
		}

		if isContainerDisk {
			logrus.Infof("container storage disk will be mount to %q", define.ContainerStorageMountPoint)
			blkDev.MountTo = define.ContainerStorageMountPoint
		}

		vmc.BlkDevs = append(vmc.BlkDevs, blkDev)
	}

	return nil
}

func (vmc *VMConfig) WithUncompressedKernel(kernelPath string) {
	vmc.Kernel = kernelPath
}

func (vmc *VMConfig) WithInitramfs(initrdPath string) {
	vmc.Initrd = initrdPath
}

func (vmc *VMConfig) WithKernelCmdline(cmdline []string) {
	vmc.KernelCmdline = cmdline
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

func (vmc *VMConfig) WithGuestAgentConfigure() error {
	if vmc.RootFS == "" {
		return fmt.Errorf("root file system is not set")
	}

	src := vmc.ExternalTools.DarwinTools.GuestAgent
	dest := vmc.RootFS + "/3rd/bin/" + guestAgent

	if err := system.CopyFile(src, dest); err != nil {
		return fmt.Errorf("failed to copy guest-agent file to rootfs: %w", err)
	}

	vmc.Cmdline.GuestAgent = vmc.ExternalTools.LinuxTools.GuestAgent

	vmc.Cmdline.Env = []string{"PATH=/3rd/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin"}

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		vmc.Cmdline.GuestAgentArgs = append(vmc.Cmdline.GuestAgentArgs, "--verbose")
	}

	logrus.Debugf("guest-agent %q args is: %q", vmc.Cmdline.GuestAgent, vmc.Cmdline.GuestAgentArgs)
	return nil
}

func (vmc *VMConfig) WithUserProvidedCmdline(bin string, args, envs []string) error {
	if strings.TrimSpace(bin) == "" {
		return fmt.Errorf("target binary is empty")
	}

	if err := validateArgs(args); err != nil {
		return err
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

func (vmc *VMConfig) WithRunMode(runMode define.RunMode) {
	vmc.RunMode = runMode.String()
}

func (vmc *VMConfig) WithBuiltInRootfs() error {
	rootfsPath, err := path.GetBuiltinRootfsPath()
	if err != nil {
		return fmt.Errorf("failed to get builtin rootfs path: %w", err)
	}

	return vmc.WithUserProvidedRootFS(rootfsPath)
}

func (vmc *VMConfig) WithContainerDataDisk(ctx context.Context, containerDataDiskPath string) error {
	return vmc.WithBlkDisk(ctx, []string{containerDataDiskPath}, true)
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

const guestAgent = "guest-agent"

func (vmc *VMConfig) WithExternalTools() error {
	libexecPath, err := path.GetLibexecPath()
	if err != nil {
		return fmt.Errorf("failed to get libexec path: %w", err)
	}

	vmc.ExternalTools.DarwinTools.MkfsExt4 = filepath.Join(libexecPath, "mkfs.ext4")
	vmc.ExternalTools.DarwinTools.Blkid = filepath.Join(libexecPath, "blkid")
	vmc.ExternalTools.DarwinTools.FsckExt4 = filepath.Join(libexecPath, "fsck.ext4")

	vmc.ExternalTools.DarwinTools.GuestAgent = filepath.Join(libexecPath, guestAgent)

	vmc.ExternalTools.LinuxTools.Busybox = "/3rd/bin/busybox"
	vmc.ExternalTools.LinuxTools.DropBear = "/3rd/bin/dropbear"
	vmc.ExternalTools.LinuxTools.DropBearKey = "/3rd/bin/dropbearkey"

	vmc.ExternalTools.LinuxTools.GuestAgent = "/3rd/bin/" + guestAgent

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

	vmc.BlkDevs = []define.BlkDev{}
	vmc.Mounts = []define.Mount{}

	return vmc
}
