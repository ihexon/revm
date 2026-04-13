//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
)

func (v *machineBuilder) configureContainerRAWDisk(ctx context.Context, spec *ContainerDiskSpec, defaultPath string) error {
	blkDev, err := v.prepareContainerStorageDisk(ctx, spec, defaultPath)
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
