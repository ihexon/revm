//go:build darwin && (arm64 || amd64)

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const virtioFSOverrideStatXattr = "user.containers.override_stat"

// EnsureVirtioFSRootOwnership records the guest-visible ownership of every
// inode in the exported directory. libkrun's macOS passthrough backend reads
// this xattr; mount uid=/gid= options are not supported by the virtiofs client.
func EnsureVirtioFSRootOwnership(path string) error {
	return filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := []byte(fmt.Sprintf("0:0:0%o", info.Mode().Perm()))
		if err := setVirtioFSOwnershipXattr(current, info.Mode().Perm(), value); err != nil {
			return fmt.Errorf("set virtiofs ownership xattr on %q: %w", current, err)
		}
		return nil
	})
}

func setVirtioFSOwnershipXattr(path string, mode os.FileMode, value []byte) error {
	original := mode.Perm()
	writable := original | 0200
	if writable != original {
		if err := os.Chmod(path, writable); err != nil {
			return err
		}
		setErr := unix.Setxattr(path, virtioFSOverrideStatXattr, value, 0)
		restoreErr := os.Chmod(path, original)
		if setErr != nil {
			return setErr
		}
		return restoreErr
	}
	return unix.Setxattr(path, virtioFSOverrideStatXattr, value, 0)
}
