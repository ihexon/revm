//go:build linux && (arm64 || amd64)

package filesystem

import (
	"fmt"
	"linuxvm/pkg/define"
	"os"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"

	"github.com/moby/sys/mount"
)

const (
	Tmpfs        = "tmpfs"
	TmpDir       = "/tmp"
	RunDir       = "/run"
	TmpMountOpts = "rw,nosuid,relatime"
	VirtioFs     = "virtiofs"
)

func MountTmpfs() error {
	dirs := []string{
		TmpDir,
		RunDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 755); err != nil {
			return fmt.Errorf("failed to create %q dir: %w", dir, err)
		}

		isMounted, err := mountinfo.Mounted(dir)
		if err != nil {
			return fmt.Errorf("failed to check %q mounted: %w", TmpDir, err)
		}

		if isMounted {
			return fmt.Errorf("%q is already mounted, can not mount again", TmpDir)
		}

		logrus.Infof("mount devices %q, type %q to %q", Tmpfs, Tmpfs, dir)
		if err = mount.Mount(Tmpfs, dir, Tmpfs, TmpMountOpts); err != nil {
			return fmt.Errorf("failed to mount /run dir: %w", err)
		}
	}

	return nil
}

// MountVirtioFS load $rootfs/.vmconfig, and mount the virtiofs mnt
func LoadVMConfigAndMountVirtioFS(file string) error {
	vmc, err := define.LoadVMCFromFile(file)
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	for _, mnt := range vmc.Mounts {
		isMounted, err := mountinfo.Mounted(mnt.Target)
		if err != nil {
			return fmt.Errorf("failed to check %q mounted: %w", mnt.Target, err)
		}

		if isMounted {
			return fmt.Errorf("can not mount host directory to guest rootfs, %q is already mounted, can not mount again", mnt.Target)
		}

		if err = os.MkdirAll(mnt.Target, 755); err != nil {
			return fmt.Errorf("failed to create virtiofs: %w", err)
		}

		logrus.Infof("mount host dir %q to guest %q, tag %q", mnt.Source, mnt.Target, mnt.Tag)
		if err = mount.Mount(mnt.Tag, mnt.Target, VirtioFs, ""); err != nil {
			return fmt.Errorf("failed to mount virtiofs: %w", err)
		}
	}

	return nil
}
