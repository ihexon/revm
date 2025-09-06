//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"os"
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
	}

	return nil
}

func (vmc *VMConfig) CreateRawDiskWhenNeeded(ctx context.Context) error {
	for _, disk := range vmc.DataDisk {
		if system.IsPathExist(disk.Path) {
			// if the disk already exist, mark it with truncate false means we want
			// reuse the disk, not create a new one.
			disk.ReUse = true
		} else {
			// if the disk not exist, mark it with truncate true means we want
			// create a new disk.
			disk.ReUse = false
		}

		// if the disk mark with truncate, recreate it
		if !disk.ReUse {
			if err := filesystem.CreateDiskAndFormatExt4(ctx, disk.Path, uuid.NewString(), true); err != nil {
				return fmt.Errorf("failed to create raw disk %q: %w", disk.Path, err)
			}
		}
	}

	return vmc.ParseDiskInfo(ctx)
}

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
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
		logrus.Infof("using system http proxy: %q", httpProxy)
		vmc.Cmdline.Env = append(vmc.Cmdline.Env, httpProxy)
	}

	if proxyInfo.HTTPS == nil {
		logrus.Warnf("no system https proxy found")
	} else {
		httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
		logrus.Infof("using system https proxy: %q", httpsProxy)
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

	// Fill the SSHInfo
	vmc.SSHInfo.GuestPort = define.DefaultGuestSSHPort
	vmc.SSHInfo.GuestAddr = define.DefaultGuestSSHAddr

	portInHostSide, err := network.GetAvailablePort()
	if err != nil {
		return fmt.Errorf("failed to get avaliable port: %w", err)
	}
	vmc.SSHInfo.HostPort = portInHostSide
	vmc.SSHInfo.HostAddr = define.DefaultSSHInHost

	vmc.SSHInfo.User = define.DefaultGuestUser

	return nil
}

func (vmc *VMConfig) Lock() (*flock.Flock, error) {
	f := filepath.Join(vmc.RootFS, define.LockFile)
	fileLock := flock.New(f)
	logrus.Infof("try to lock file: %s", f)
	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}

	if !ifLocked {
		return nil, fmt.Errorf("try lock file unsuccessful, mybe there is another vm instance running")
	}

	return fileLock, nil
}
