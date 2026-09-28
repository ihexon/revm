package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/protocol"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type Mnt struct {
	ReadOnly bool
	Source   string
	Tag      string
	Target   string
	Type     string
	Opts     string
	UUID     string
}

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
	var mountOpts []string
	if mnt.Opts != "" {
		mountOpts = append(mountOpts, mnt.Opts)
	}
	if mnt.ReadOnly {
		mountOpts = append(mountOpts, "ro")
		// A read-only block device cannot replay an ext4 journal. noload keeps
		// the mount strictly read-only while allowing a clean image to mount.
		if mnt.Type == "ext4" {
			mountOpts = append(mountOpts, "noload")
		}
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
		// Keep journal ordering for writable ext4. Read-only mounts use noload
		// above because the block device itself is read-only.
		if mnt.Type == "ext4" && !mnt.ReadOnly {
			mountOpts = append(mountOpts, "data=ordered")
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
	if len(mountOpts) > 0 {
		// Alpine's mount implementation treats repeated -o flags inconsistently.
		// Pass one comma-separated option list so read-only and discard semantics survive.
		args = append([]string{"-o", strings.Join(mountOpts, ",")}, args...)
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
			Target: "/proc",
			Opts:   "rw,nosuid,nodev,noexec,relatime",
			Type:   procType,
		},
		{
			Target: "/mnt",
			Type:   tmpfsType,
		},
		{
			Target: "/tmp",
			Type:   tmpfsType,
		},
		{
			Target: "/run",
			Type:   tmpfsType,
		},
		{
			Target: "/var",
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
		//{
		//	Target: "/sys/kernel/config",
		//	Opts:   "rw,nosuid,nodev,noexec,relatime",
		//	Type:   configfsType,
		//},
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

func MountVirtiofs(ctx context.Context, vmc *protocol.GuestSpec) error {
	if len(vmc.Storage.VirtioFS) == 0 {
		logrus.Debug("no virtiofs mounts configured")
		return nil
	}

	for _, virtiofsMnt := range vmc.Storage.VirtioFS {
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

func MountBlockDevices(ctx context.Context, vmc *protocol.GuestSpec) error {
	if len(vmc.Storage.Blocks) == 0 {
		logrus.Debug("no block devices will be mounted, skip")
		return nil
	}

	for _, dataDiskMnt := range vmc.Storage.Blocks {
		mnt := &Mnt{
			Source:   dataDiskMnt.Path,
			Opts:     "discard",
			UUID:     dataDiskMnt.UUID,
			Type:     dataDiskMnt.FsType,
			Target:   dataDiskMnt.MountTo,
			ReadOnly: dataDiskMnt.ReadOnly,
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
