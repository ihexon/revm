//go:build darwin && arm64

package filesystem

import "context"

func MountTmpfs() error {
	return nil
}

// LoadVMConfigAndMountVirtioFS load $rootfs/.vmconfig, and mount the virtiofs mnt
func LoadVMConfigAndMountVirtioFS(ctx context.Context) error {
	return nil
}
