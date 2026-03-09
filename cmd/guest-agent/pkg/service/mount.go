package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

type Mnt define.Mount

// virtiofs filesystem type
const virtiofsType = "virtiofs"

// pseudo filesystem type
const (
	tmpfsType        = "tmpfs"
	devtmpfsType     = "devtmpfs"
	devpts           = "devpts"
	procType         = "proc"
	sysfsType        = "sysfs"
	cgroup2Type      = "cgroup2"
	configfsType     = "configfs"
	bpfFsType        = "bpf"
	binfmtMiscFsType = "binfmt_misc"
	fusectl          = "fusectl"
)

type MountActionType int

const (
	UUIDAction MountActionType = iota
	VirtioFsAction
	PseudoFsAction
)

func (mnt *Mnt) makeMountCmdline(action MountActionType) ([]string, error) {
	var args []string

	if mnt.Opts != "" {
		args = append(args, "-o", mnt.Opts)
	}

	if mnt.ReadOnly {
		args = append(args, "-o", "ro")
	}

	switch action {
	case UUIDAction:
		// UUIDAction require filesystem has UUID
		if mnt.UUID == "" {
			return nil, fmt.Errorf("UUID is empty")
		}
		if mnt.Type == "" {
			return nil, fmt.Errorf("filesystem type is empty")
		}
		// only ext4 support data=ordered
		if mnt.Type == "ext4" {
			args = append(args, "-o", "data=ordered")
		}
		args = append(args, "-t", mnt.Type, "UUID="+mnt.UUID, mnt.Target)
	case VirtioFsAction:
		// virtiofs mount by tag, but also require filesystem type is virtiofs
		if mnt.Type != virtiofsType {
			return nil, fmt.Errorf("filesystem type is not virtiofs")
		}
		if mnt.Tag == "" {
			return nil, fmt.Errorf("virtio tag is empty")
		}
		args = append(args, "-t", mnt.Type, mnt.Tag, mnt.Target)
	case PseudoFsAction:
		// pseudo filesystem mount by type
		if mnt.Type == "" {
			return nil, fmt.Errorf("pseudo filesystem type is empty")
		}
		args = append(args, "-t", mnt.Type, mnt.Type, mnt.Target)
	default:
		return nil, fmt.Errorf("unsupported mount action")
	}

	return args, nil
}

func (mnt *Mnt) Mount(ctx context.Context, action MountActionType) error {
	if err := os.MkdirAll(mnt.Target, 0755); err != nil {
		return fmt.Errorf("failed to create dir for mount point: %w", err)
	}

	args, err := mnt.makeMountCmdline(action)
	if err != nil {
		return fmt.Errorf("mount %s: %w", mnt.Target, err)
	}
	return Mount(ctx, args...)
}

func MountAllPseudoMnt(ctx context.Context) error {
	var pseudoMnts = []Mnt{
		{
			Target: "/tmp",
			Type:   tmpfsType,
		},
		{
			Target: "/run",
			Type:   tmpfsType,
		},
		{
			Target: "/var/tmp",
			Type:   tmpfsType,
		},
		{
			Target: "/disk_mnt",
			Type:   tmpfsType,
		},
		{
			Target: "/dev",
			Opts:   "rw,nosuid,noexec,relatime",
			Type:   devtmpfsType,
		},
		{
			Target: "/dev/pts",
			Opts:   "rw,nosuid,noexec,relatime,mode=600,ptmxmode=000",
			Type:   devpts,
		},
		{
			Target: "/dev/shm",
			Type:   tmpfsType,
		},
		{
			Target: "/proc",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   procType,
		},
		{
			Target: "/proc/sys/fs/binfmt_misc",
			Type:   binfmtMiscFsType,
		},
		{
			Target: "/sys",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   sysfsType,
		},
		{
			Target: "/sys/fs/fuse/connections",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   fusectl,
		},
		{
			Target: "/sys/fs/cgroup",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   cgroup2Type,
		},
		{
			Target: "/sys/fs/bpf",
			Opts:   "rw,nosuid,nodev,noexec,relatime,mode=700",
			Type:   bpfFsType,
		},
		{
			Target: "/sys/kernel/config",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   configfsType,
		},
	}

	for _, mnt := range pseudoMnts {
		if IsMounted(mnt.Target) {
			logrus.Debugf("mount point %q is already mounted, skip", mnt.Target)
			continue
		}

		if err := mnt.Mount(ctx, PseudoFsAction); err != nil {
			return fmt.Errorf("mount %s: %w", mnt.Target, err)
		}
	}

	return nil
}

func MountVirtiofs(ctx context.Context, vmc *define.Machine) error {
	if len(vmc.Mounts) == 0 {
		logrus.Debug("no virtiofs mounts configured")
		return nil
	}

	for _, virtiofsMnt := range vmc.Mounts {
		mnt := &Mnt{
			Tag:      virtiofsMnt.Tag,
			Target:   virtiofsMnt.Target,
			Type:     virtiofsMnt.Type,
			ReadOnly: virtiofsMnt.ReadOnly,
		}

		if IsMounted(mnt.Target) {
			logrus.Debugf("mount point %s already mounted, skip", mnt.Target)
			continue
		}

		logrus.Infof("mounting virtiofs %s to %s", mnt.Tag, mnt.Target)
		if err := mnt.Mount(ctx, VirtioFsAction); err != nil {
			return fmt.Errorf("mount virtio-fs failed: %q: %w", mnt.Target, err)
		}
	}

	return nil
}

func MountBlockDevices(ctx context.Context, vmc *define.Machine) error {
	if len(vmc.BlkDevs) == 0 {
		logrus.Debug("no block devices will be mounted, skip")
		return nil
	}

	for _, dataDiskMnt := range vmc.BlkDevs {
		mnt := &Mnt{
			Source: dataDiskMnt.Path,
			Opts:   "rw,discard",
			UUID:   dataDiskMnt.UUID,
			Type:   dataDiskMnt.FsType,
			Target: dataDiskMnt.MountTo,
		}

		if IsMounted(mnt.Target) {
			logrus.Debugf("mount point %s already mounted, skip", mnt.Target)
			continue
		}

		logrus.Infof("mounting block device %s to %s", mnt.Source, mnt.Target)
		if err := mnt.Mount(ctx, UUIDAction); err != nil {
			return fmt.Errorf("mount block device %s: %w", mnt.Source, err)
		}
	}

	return nil
}

// SetupContainerStorage finds the container disk in vmc.BlkDevs (by ContainerDiskUUID),
// removes /var/lib/containers, and creates a symlink /var/lib/containers -> /mnt/<UUID>.
func SetupContainerStorage(vmc *define.Machine) error {
	var mountPoint string
	for _, dev := range vmc.BlkDevs {
		if dev.UUID == define.ContainerDiskUUID {
			mountPoint = dev.MountTo
			break
		}
	}
	if mountPoint == "" {
		return fmt.Errorf("container disk (UUID %s) not found in block devices", define.ContainerDiskUUID)
	}

	if err := os.MkdirAll(filepath.Dir(define.ContainerStorageMountPoint), 0755); err != nil {
		return fmt.Errorf("failed to create parent dir for container storage: %w", err)
	}

	if err := os.RemoveAll(define.ContainerStorageMountPoint); err != nil {
		return fmt.Errorf("failed to remove %s: %w", define.ContainerStorageMountPoint, err)
	}

	if err := os.Symlink(mountPoint, define.ContainerStorageMountPoint); err != nil {
		return fmt.Errorf("failed to symlink %s -> %s: %w", define.ContainerStorageMountPoint, mountPoint, err)
	}

	return nil
}
