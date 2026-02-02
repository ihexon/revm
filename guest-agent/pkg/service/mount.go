package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"

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

func IsMounted(target string) bool {
	if err := Busybox.Exec(context.Background(), "mountpoint", target); err != nil {
		return false
	}
	return true
}

func (mnt *Mnt) makeMountCmdline(action MountActionType) ([]string, error) {
	args := []string{"mount"}

	if mnt.Opts != "" {
		args = append(args, "-o", mnt.Opts)
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

func Umount(ctx context.Context, target string) error {
	return Busybox.Exec(ctx, "umount", "-l", "-d", "-f", target)
}

func (mnt *Mnt) Mount(ctx context.Context, action MountActionType) error {
	if err := os.MkdirAll(mnt.Target, 0755); err != nil {
		return fmt.Errorf("failed to create dir for mount point: %w", err)
	}

	args, err := mnt.makeMountCmdline(action)
	if err != nil {
		return err
	}
	return Busybox.Exec(ctx, args...)
}

func MountAllPseudoMnt(ctx context.Context) error {
	var pseudoMnts = []Mnt{
		{
			Target:       "/tmp",
			Type:         tmpfsType,
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
			logrus.Warnf("mount point %q is already mounted, skip", mnt.Target)
			continue
		}

		if err := mnt.Mount(ctx, PseudoFsAction); err != nil {
			return err
		}
	}

	return nil
}

func MountVirtiofs(ctx context.Context, vmc *define.VMConfig) error {
	for _, virtiofsMnt := range vmc.Mounts {
		mnt := &Mnt{
			Tag:    virtiofsMnt.Tag,
			Target: virtiofsMnt.Target,
			Type:   virtiofsMnt.Type,
		}

		if IsMounted(mnt.Target) {
			logrus.Warnf("mount point %q is already mounted, skip", mnt.Target)
			continue
		}

		if err := mnt.Mount(ctx, VirtioFsAction); err != nil {
			return fmt.Errorf("mount virtio-fs failed: %q: %w", mnt.Target, err)
		}
	}

	return nil
}

func MountBlockDevices(ctx context.Context, vmc *define.VMConfig) error {
	for _, dataDiskMnt := range vmc.BlkDevs {
		mnt := &Mnt{
			Opts:   "rw,discard",
			UUID:   dataDiskMnt.UUID,
			Type:   dataDiskMnt.FsType,
			Target: dataDiskMnt.MountTo,
		}

		if IsMounted(mnt.Target) {
			logrus.Warnf("mount point %q is already mounted, skip", mnt.Target)
			continue
		}

		if err := mnt.Mount(ctx, UUIDAction); err != nil {
			return err
		}
	}

	return nil
}
