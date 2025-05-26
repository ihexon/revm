package filesystem

import (
	"fmt"
	"github.com/moby/sys/mount"
	"github.com/moby/sys/mountinfo"
	"os"
)

const (
	Tmpfs        = "tmpfs"
	TmpDir       = "/tmp"
	TmpMountOpts = "rw,nosuid,relatime"
)

func MountTmpfs() error {
	isMounted, err := mountinfo.Mounted(TmpDir)
	if err != nil {
		return fmt.Errorf("failed to check %q mounted: %w", TmpDir, err)
	}
	if isMounted {
		return fmt.Errorf("%q is already mounted, can not mount again", TmpDir)
	}

	if err = os.MkdirAll(TmpDir, 755); err != nil {
		return fmt.Errorf("failed to create tmp dir: %w", err)
	}

	return mount.Mount(Tmpfs, TmpDir, Tmpfs, TmpMountOpts)
}
