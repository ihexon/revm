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
