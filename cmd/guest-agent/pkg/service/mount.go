package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/protocol"
	"os"

	"github.com/sirupsen/logrus"
)

func mountFS(ctx context.Context, source, target, fsType, options string) error {
	if IsMounted(target) {
		return nil
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	logrus.Infof("mounting %s to %s", source, target)
	if err := Mount(ctx, "-t", fsType, "-o", options, source, target); err != nil {
		return fmt.Errorf("mount %s: %w", target, err)
	}
	return nil
}

func MountAllPseudoMnt(ctx context.Context) error {
	for _, m := range []struct{ target, fsType, options string }{
		{"/proc", "proc", "rw,nosuid,nodev,noexec,relatime"},
		{"/mnt", "tmpfs", "rw"},
		{"/tmp", "tmpfs", "rw"},
		{"/run", "tmpfs", "rw"},
		{"/var", "tmpfs", "rw"},
		{"/dev", "devtmpfs", "rw,nosuid,noexec,relatime"},
		{"/dev/pts", "devpts", "rw,nosuid,noexec,relatime,mode=600,ptmxmode=000"},
		{"/dev/shm", "tmpfs", "rw"},
		{"/proc/sys/fs/binfmt_misc", "binfmt_misc", "rw"},
		{"/sys", "sysfs", "rw,nosuid,nodev,noexec,relatime"},
		{"/sys/fs/fuse/connections", "fusectl", "rw,nosuid,nodev,noexec,relatime"},
		{"/sys/fs/cgroup", "cgroup2", "rw,nosuid,nodev,noexec,relatime"},
		{"/sys/fs/bpf", "bpf", "rw,nosuid,nodev,noexec,relatime,mode=700"},
	} {
		if err := mountFS(ctx, m.fsType, m.target, m.fsType, m.options); err != nil {
			return err
		}
	}
	return nil
}

func MountVirtiofs(ctx context.Context, vmc *protocol.GuestSpec) error {
	for _, m := range vmc.Storage.VirtioFS {
		options := "rw"
		if m.ReadOnly {
			options = "ro"
		}
		if err := mountFS(ctx, m.Tag, m.Target, "virtiofs", options); err != nil {
			return err
		}
	}
	return nil
}

func MountBlockDevices(ctx context.Context, vmc *protocol.GuestSpec) error {
	for _, m := range vmc.Storage.Blocks {
		options := "discard"
		if m.ReadOnly {
			options += ",ro"
			// An ext4 journal cannot be replayed on a read-only device.
			if m.FsType == "ext4" {
				options += ",noload"
			}
		} else if m.FsType == "ext4" {
			options += ",data=ordered"
		}
		if err := mountFS(ctx, "UUID="+m.UUID, m.MountTo, m.FsType, options); err != nil {
			return err
		}
	}
	return nil
}
