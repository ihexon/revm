//go:build linux && (arm64 || amd64)

package filesystem

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type XattrManager interface {
	SetXattr(ctx context.Context, filePath string, key string, value string, overwrite bool) error
	GetXattr(ctx context.Context, filePath string, key string) (string, error)
}

type xattrManager struct{}

func NewXattrManager() XattrManager {
	return &xattrManager{}
}

func (b xattrManager) SetXattr(ctx context.Context, blkPath string, namespace string, value string, overwrite bool) error {
	if namespace == "" {
		return fmt.Errorf("xattr key cannot be empty")
	}

	if !strings.HasPrefix(namespace, "user.vm.") {
		return fmt.Errorf("xattr key must start with \"user.vm.\", got %q", namespace)
	}

	if value == "" {
		return fmt.Errorf("xattr value cannot be empty for key %q", namespace)
	}

	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return err
	}

	existValue, _ := b.GetXattr(ctx, blkPath, namespace)
	if existValue != "" && !overwrite {
		return nil
	}

	return unix.Setxattr(blkPath, namespace, []byte(value), 0)
}

func (b xattrManager) GetXattr(ctx context.Context, blkPath string, namespace string) (string, error) {
	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return "", err
	}

	// First call to get size
	sz, err := unix.Getxattr(blkPath, namespace, nil)
	if err != nil {
		return "", fmt.Errorf("getxattr %q on %q: %w", namespace, blkPath, err)
	}

	buf := make([]byte, sz)
	_, err = unix.Getxattr(blkPath, namespace, buf)
	if err != nil {
		return "", fmt.Errorf("getxattr %q on %q (read %d bytes): %w", namespace, blkPath, sz, err)
	}

	return string(buf), nil
}
