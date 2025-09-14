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

func mountUUIDDevice(ctx context.Context, uuid, mountPoint, fstype string) error {
	if mountPoint == "" {
		return fmt.Errorf("mount point is empty")
	}

	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("failed to create dir for mount point: %w", err)
	}

	mounted, err := mountinfo.Mounted(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to check %q mounted: %w", mountPoint, err)
	}

	if mounted {
		return fmt.Errorf("target mount point %q is already mounted, skip", mountPoint)
	}

	cmd := exec.CommandContext(ctx, "busybox.static", "mount", "-t", fstype, "-o", "data=ordered,discard", "UUID="+uuid, mountPoint)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = nil
	logrus.Debugf("mount disk with uuid %q to %q", uuid, mountPoint)
	return cmd.Run()
}

func LoadVMConfigAndMountDataDisk(ctx context.Context) error {
	vmc, err := define.LoadVMCFromFile(filepath.Join("/", define.VMConfigFile))
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	for _, disk := range vmc.DataDisk {
		logrus.Infof("mount disk %q to %q", disk.Path, disk.MountPoint)
		if err = mountUUIDDevice(ctx, disk.UUID, disk.MountPoint, disk.FileSystemType); err != nil {
			return fmt.Errorf("failed to mount disk %q: %w", disk.Path, err)
		}
	}

	return nil
}
