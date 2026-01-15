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
	ssh "linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

type (
	VMConfig define.VMConfig
)

func (v *VMConfig) WithSystemProxy() error {
	proxyInfo, err := network.GetAndNormalizeSystemProxy()
	if err != nil {
		return fmt.Errorf("failed to get and normalize system proxy: %w", err)
	}

	if proxyInfo.HTTP == nil {
		logrus.Warnf("no system http proxy found")
	} else {
		httpProxy := fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port)
		logrus.Infof("using http proxy: %q", httpProxy)
		v.Cmdline.Env = append(v.Cmdline.Env, httpProxy)
	}

	if proxyInfo.HTTPS == nil {
		logrus.Warnf("no system https proxy found")
	} else {
		httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
		logrus.Infof("using https proxy: %q", httpsProxy)
		v.Cmdline.Env = append(v.Cmdline.Env, httpsProxy)
	}

	return nil
}

// GenerateSSHInfo Generate SSH info for the VM, notice the ssh keypair will be written when guest rootfs actually running.
func (v *VMConfig) GenerateSSHInfo() error {
	logrus.Infof("ssh keypair write to: %q", v.SSHInfo.HostSSHKeyPairFile)
	keyPair, err := ssh.GenerateKeyPair(v.SSHInfo.HostSSHKeyPairFile, ssh.DefaultKeyGenOptions())
	if err != nil {
		return fmt.Errorf("failed to generate host ssh keypair for host: %w", err)
	}

	// Fill the ssh keypair into vmc.SSHInfo using the keypair in memory
	v.SSHInfo.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	v.SSHInfo.HostSSHPublicKey = keyPair.AuthorizedKey()

	return nil
}

func (v *VMConfig) Lock() (*flock.Flock, error) {
	if v.RootFS == "" {
		return nil, fmt.Errorf("root file system is not set")
	}

	fileLock := flock.New(filepath.Join(v.RootFS, define.LockFile))

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

func (v *VMConfig) WithBlkDisk(ctx context.Context, blocks []string, isContainerDisk bool) error {
	diskMgr := disk.NewBlkManager(
		v.ExternalTools.DarwinTools.Mke2fs,
		v.ExternalTools.DarwinTools.Blkid,
		v.ExternalTools.DarwinTools.FsckExt4,
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

			if err := diskMgr.Format(ctx, absPath, "ext4"); err != nil {
				return fmt.Errorf("failed to format ext4 disk: %w", err)
			}

			if err := diskMgr.FsCheck(ctx, absPath); err != nil {
				return fmt.Errorf("filesystem integrity check %q error %w", absPath, err)
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

		v.BlkDevs = append(v.BlkDevs, blkDev)
	}

	return nil
}

func (v *VMConfig) WithUncompressedKernel(kernelPath string) {
	v.Kernel = kernelPath
}

func (v *VMConfig) WithInitramfs(initrdPath string) {
	v.Initrd = initrdPath
}

func (v *VMConfig) WithKernelCmdline(cmdline []string) {
	v.KernelCmdline = cmdline
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

func (v *VMConfig) WithGuestAgentConfigure() error {
	if v.RootFS == "" {
		return fmt.Errorf("root file system is not set")
	}

	src := v.ExternalTools.DarwinTools.GuestAgent
	dest := v.RootFS + "/3rd/bin/" + guestAgent

	if err := system.CopyFile(src, dest); err != nil {
		return fmt.Errorf("failed to copy guest-agent file to rootfs: %w", err)
	}

	v.Cmdline.GuestAgent = v.ExternalTools.LinuxTools.GuestAgent

	v.Cmdline.Env = append(v.Cmdline.Env, "PATH=/3rd/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin")

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		v.Cmdline.GuestAgentArgs = append(v.Cmdline.GuestAgentArgs, "--verbose")
	}

	logrus.Debugf("guest-agent %q args is: %q", v.Cmdline.GuestAgent, v.Cmdline.GuestAgentArgs)
	return nil
}

func (v *VMConfig) WithUserProvidedCmdline(bin string, args, envs []string) error {
	if strings.TrimSpace(bin) == "" {
		return fmt.Errorf("target binary is empty")
	}

	if err := validateArgs(args); err != nil {
		return err
	}

	v.Cmdline.Workspace = define.DefaultWorkDir
	v.Cmdline.TargetBin = bin
	v.Cmdline.TargetBinArgs = args
	v.Cmdline.Env = append(v.Cmdline.Env, envs...)

	return nil
}

func (v *VMConfig) WithResources(memory uint64, cpus int8) {
	if cpus <= 0 {
		logrus.Warnf("1 cpu cores is the minimum value and has been enforced")
		cpus = 1
	}

	if memory <= 512 {
		logrus.Warnf("512MB of memory is the minimum value and has been enforced")
		memory = 512
	}

	v.MemoryInMB = memory
	v.Cpus = cpus
}

func (v *VMConfig) WithShareUserHomeDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("can not get user home directory: %w", err)
	}

	return v.WithUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *VMConfig) WithUserProvidedMounts(dirs []string) error {
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

func (v *VMConfig) WithUserProvidedRootFS(rootfsPath string) error {
	if rootfsPath == "" {
		return fmt.Errorf("rootfs path is empty")
	}
	rootfsPath, err := filepath.Abs(rootfsPath)
	if err != nil {
		return err
	}

	v.RootFS = rootfsPath

	if _, err = v.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock for rootfs: %v", err)
	}

	return nil
}

func (v *VMConfig) WithRunMode(runMode define.RunMode) {
	v.RunMode = runMode.String()
}

func (v *VMConfig) WithBuiltInRootfs() error {
	rootfsPath, err := path.GetBuiltinRootfsPath()
	if err != nil {
		return fmt.Errorf("failed to get builtin rootfs path: %w", err)
	}

	return v.WithUserProvidedRootFS(rootfsPath)
}

func (v *VMConfig) WithContainerDataDisk(ctx context.Context, containerDataDiskPath string) error {
	return v.WithBlkDisk(ctx, []string{containerDataDiskPath}, true)
}

func (v *VMConfig) WithPodmanListenAPIInHost(listenAPIPath string) error {
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

	v.PodmanInfo = define.PodmanInfo{
		UnixSocksAddr: unixAddr.String(),
	}
	return nil
}

// WithRESTAPIAddress set the REST API address for the VM. only support unix socket
func (v *VMConfig) WithRESTAPIAddress(path string) error {
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

	v.RestAPIAddress = unixAddr.String()
	return nil
}

const guestAgent = "guest-agent"

func (v *VMConfig) WithExternalTools() error {
	libexecPath, err := path.GetLibexecPath()
	if err != nil {
		return fmt.Errorf("failed to get libexec path: %w", err)
	}

	v.ExternalTools.DarwinTools.Mke2fs = filepath.Join(libexecPath, "mke2fs")
	v.ExternalTools.DarwinTools.Blkid = filepath.Join(libexecPath, "blkid")
	v.ExternalTools.DarwinTools.FsckExt4 = filepath.Join(libexecPath, "fsck.ext4")

	v.ExternalTools.DarwinTools.GuestAgent = filepath.Join(libexecPath, guestAgent)

	v.ExternalTools.LinuxTools.Busybox = "/3rd/bin/busybox"
	v.ExternalTools.LinuxTools.DropBear = "/3rd/bin/dropbearmulti"

	v.ExternalTools.LinuxTools.GuestAgent = "/3rd/bin/" + guestAgent

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

		GuestPodmanReadyChan:    make(chan struct{}, 1),
		GuestSSHServerReadyChan: make(chan struct{}, 1),
	}

	vmc.BlkDevs = []define.BlkDev{}
	vmc.Mounts = []define.Mount{}

	return vmc
}
