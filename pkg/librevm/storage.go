//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
)

func (v *machineBuilder) configureContainerRAWDisk(ctx context.Context, diskPath string, version string) error {
	blkDev, err := v.prepareRawDisk(ctx, RawDiskSpec{
		Path:    diskPath,
		UUID:    define.ContainerDiskUUID,
		Version: version,
		MountTo: define.ContainerStorageMountPoint,
	})
	if err != nil {
		return fmt.Errorf("prepare container storage raw disk: %w", err)
	}

	v.BlkDevs = append(v.BlkDevs, blkDev)
	return nil
}

func (v *machineBuilder) withUserProvidedStorageRAWDisk(ctx context.Context, disks []RawDiskSpec) error {
	for _, spec := range disks {
		blkDev, err := v.prepareRawDisk(ctx, spec)
		if err != nil {
			return fmt.Errorf("prepare raw disk %q: %w", spec.Path, err)
		}

		v.BlkDevs = append(v.BlkDevs, blkDev)
	}

	return nil
}
