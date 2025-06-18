package filesystem

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/moby/sys/mount"
	"github.com/moby/sys/mountinfo"
)

const (
	Tmpfs        = "tmpfs"
	TmpDir       = "/tmp"
	TmpMountOpts = "rw,nosuid,relatime"

	VirtioFs = "virtiofs"
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
