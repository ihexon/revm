package filesystem

import (
	"encoding/json"
	"fmt"
	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
	"os"

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

// VMConfig taken from `pkg/vmconfig/vmconfig.go`
type VMConfig struct {
	CtxID      uint32
	MemoryInMB int32
	Cpus       int8
	RootFS     string

	// data disk will map into /dev/vdX
	DataDisk []string
	// GVproxy control endpoint
	GVproxyEndpoint string
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string
	LogLevel            string
	Mounts              []Mount
}

// MountVirtioFS load $rootfs/.vmconfig, and mount the virtiofs mnt
func MountVirtioFS(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()

	vmc := &VMConfig{}

	if err = json.NewDecoder(f).Decode(vmc); err != nil {
		return fmt.Errorf("failed to decode file %s: %w", file, err)
	}

	for _, mnt := range vmc.Mounts {
		if err := os.MkdirAll(mnt.Target, 755); err != nil {
			return fmt.Errorf("failed to create virtiofs: %w", err)
		}

		if err := mount.Mount(mnt.Tag, mnt.Target, VirtioFs, ""); err != nil {
			return fmt.Errorf("failed to mount virtiofs: %w", err)
		}
	}

	return nil
}
