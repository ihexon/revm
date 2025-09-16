//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
)

type Mnt define.Mount

type MountActionType int

const (
	DataDiskAction MountActionType = iota
	VirtioFsAction
	PseudoFsAction
)

const (
	virtiofsType = "virtiofs"

	tmpfsType        = "tmpfs"
	devtmpfsType     = "devtmpfs"
	devpts           = "devpts"
	procType         = "proc"
	sysfsType        = "sysfs"
	cgroup2Type      = "cgroup2"
	configfsType     = "configfs"
	bpfFsType        = "bpf"
	binfmtMiscFsType = "binfmt_misc"
)

var (
	ErrMounted = errors.New("mount point is already mounted")
)

func (mnt *Mnt) makeMountCmdline(ctx context.Context, action MountActionType) (*exec.Cmd, error) {
	if err := os.MkdirAll(mnt.Target, 0755); err != nil {
		return nil, fmt.Errorf("failed to create dir for mount point: %w", err)
	}

	mounted, err := mountinfo.Mounted(mnt.Target)
	if err != nil {
		return nil, fmt.Errorf("failed to check %q mounted: %w", mnt.Target, err)
	}

	if mounted {
		logrus.Debugf("mount point %q is already mounted", mnt.Target)
		return nil, ErrMounted
	}

	if mnt.Target == "" {
		return nil, fmt.Errorf("mount point is empty")
	}

	mounter := exec.CommandContext(ctx, "busybox.static", "mount")
	mounter.Stdout = os.Stdout
	mounter.Stderr = os.Stderr
	mounter.Stdin = nil

	if mnt.Opts != "" {
		mounter.Args = append(mounter.Args, "-o", mnt.Opts)
	}

	switch action {
	case DataDiskAction:
		if mnt.UUID == "" {
			return nil, fmt.Errorf("UUID is empty")
		}
		if mnt.Type == "" {
			return nil, fmt.Errorf("filesystem type is empty")
		}
		mounter.Args = append(mounter.Args, "-t", mnt.Type, "UUID="+mnt.UUID, mnt.Target)
	case VirtioFsAction:
		if mnt.Type != virtiofsType {
			return nil, fmt.Errorf("filesystem type is not virtiofs")
		}
		if mnt.Tag == "" {
			return nil, fmt.Errorf("virtio tag is empty")
		}
		mounter.Args = append(mounter.Args, "-t", mnt.Type, mnt.Tag, mnt.Target)
	case PseudoFsAction:
		if mnt.Type == "" {
			return nil, fmt.Errorf("pseudo filesystem type is empty")
		}
		mounter.Args = append(mounter.Args, "-t", mnt.Type, mnt.Type, mnt.Target)
	default:
		return nil, fmt.Errorf("unsupport mount action")
	}

	logrus.Debugf("cmdline: %q", mounter.Args)
	return mounter, nil
}

func (mnt *Mnt) Mount(ctx context.Context, action MountActionType) error {
	mounter, err := mnt.makeMountCmdline(ctx, action)
	if err != nil {
		return err
	}
	return mounter.Run()
}

func (mnt *Mnt) Unmount(ctx context.Context) error {
	mounter := exec.CommandContext(ctx, "busybox.static", "umount", "-l", "-d", mnt.Target)
	mounter.Stderr = os.Stderr
	mounter.Stdout = os.Stdout
	mounter.Stdin = nil
	return mounter.Run()
}

func MountPseudoFilesystem(ctx context.Context) error {
	tmpMnts := []*Mnt{
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
			Type:   "fusectl",
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

	for _, mnt := range tmpMnts {
		if err := mnt.Mount(ctx, PseudoFsAction); err != nil && !errors.Is(err, ErrMounted) {
			return err
		}
	}

	return nil
}

func MountDataDisk(ctx context.Context, vmc *define.VMConfig) error {
	for _, dataDiskMnt := range vmc.DataDisk {
		mnt := &Mnt{
			UUID:   dataDiskMnt.UUID,
			Type:   dataDiskMnt.FileSystemType,
			Target: dataDiskMnt.MountPoint,
		}

		if err := mnt.Mount(ctx, DataDiskAction); err != nil {
			return err
		}
	}

	return nil
}

func MountHostDir(ctx context.Context, vmc *define.VMConfig) error {
	for _, virtiofsMnt := range vmc.Mounts {
		mnt := &Mnt{
			Tag:    virtiofsMnt.Tag,
			Target: virtiofsMnt.Target,
			Type:   virtiofsMnt.Type,
		}

		if err := mnt.Mount(ctx, VirtioFsAction); err != nil {
			return err
		}
	}
	return nil
}
