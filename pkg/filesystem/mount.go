//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package filesystem

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
)

func LoadVMConfigAndMountDataDisk(ctx context.Context) error {
	vmc, err := define.LoadVMCFromFile(filepath.Join("/", define.VMConfigFile))
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	// /var/tmp/data_disks
	prefixDir := filepath.Join("/", "var", "tmp", "data_disk")

	for _, disk := range vmc.DataDisk {
		targetMountPoint := filepath.Join(prefixDir, disk.Path)
		if err = os.MkdirAll(targetMountPoint, 0755); err != nil {
			return fmt.Errorf("failed to create dir for mount point: %w", err)
		}

		mounted, err := mountinfo.Mounted(targetMountPoint)
		if err != nil {
			return fmt.Errorf("failed to check %q mounted: %w", targetMountPoint, err)
		}
		if mounted {
			logrus.Warnf("target mount point %q is already mounted, skip", targetMountPoint)
			continue
		}

		cmd := exec.CommandContext(ctx, "busybox.static", "mount", "UUID="+disk.UUID, targetMountPoint)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Stdin = nil
		logrus.Infof("mount disk %q to %q", disk.UUID, targetMountPoint)
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("failed to mount disk: %w", err)
		}
	}

	return nil
}
