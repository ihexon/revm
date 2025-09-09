//go:build linux && (arm64 || amd64)

package filesystem

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"

	"github.com/moby/sys/mount"
)

const (
	Tmpfs        = "tmpfs"
	TmpDir       = "/tmp"
	RunDir       = "/run"
	VarTmpDir    = "/var/tmp"
	MntTmp       = "/disk_mnt"
	TmpMountOpts = "rw,nosuid,relatime"
	VirtioFs     = "virtiofs"
)

func MountTmpfs() error {
	dirs := []string{
		TmpDir,
		RunDir,
		VarTmpDir,
	}

	for _, dir := range dirs {
		logrus.Debugf("make dir %q", dir)
		if err := os.MkdirAll(dir, 755); err != nil {
			return fmt.Errorf("failed to create %q dir: %w", dir, err)
		}

		logrus.Debugf("check %q is mounted", dir)
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
func LoadVMConfigAndMountVirtioFS(ctx context.Context) error {
	vmconfigFile := filepath.Join("/", define.VMConfigFile)
	logrus.Debugf("load vmconfig.json from %q", vmconfigFile)
	vmc, err := define.LoadVMCFromFile(vmconfigFile)
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	for _, mnt := range vmc.Mounts {
		logrus.Debugf("check %q is mounted", mnt.Target)
		isMounted, err := mountinfo.Mounted(mnt.Target)
		if err != nil {
			return fmt.Errorf("failed to check %q mounted: %w", mnt.Target, err)
		}

		if isMounted {
			return fmt.Errorf("can not mount host directory to guest rootfs, %q is already mounted, can not mount again", mnt.Target)
		}

		logrus.Debugf("make dir %q", mnt.Target)
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
