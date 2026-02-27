//go:build linux && (arm64 || amd64)

package filesystem

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type XATTRManager interface {
	WriteXATTR(ctx context.Context, filePath string, key string, value string, overwrite bool) error
	GetXATTR(ctx context.Context, filePath string, key string) (string, error)
}

type xattrManager struct{}

func NewXATTRManager() XATTRManager {
	return &xattrManager{}
}

func (b xattrManager) WriteXATTR(ctx context.Context, blkPath string, namespace string, value string, overwrite bool) error {
	if namespace == "" {
		return fmt.Errorf("xattr namespace cannot be empty")
	}

	if !strings.HasPrefix(namespace, "user.vm.") {
		return fmt.Errorf("only allow write xattr with namespace starting with user.vm.*, but got: %q", namespace)
	}

	if value == "" {
		return fmt.Errorf("xattr value cannot be empty")
	}

	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return err
	}

	existValue, _ := b.GetXATTR(ctx, blkPath, namespace)
	if existValue != "" && !overwrite {
		return nil
	}

	return unix.Setxattr(blkPath, namespace, []byte(value), 0)
}

func (b xattrManager) GetXATTR(ctx context.Context, blkPath string, namespace string) (string, error) {
	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return "", err
	}

	// First call to get size
	sz, err := unix.Getxattr(blkPath, namespace, nil)
	if err != nil {
		return "", fmt.Errorf("get xattr error: %w", err)
	}

	buf := make([]byte, sz)
	_, err = unix.Getxattr(blkPath, namespace, buf)
	if err != nil {
		return "", fmt.Errorf("get xattr error: %w", err)
	}

	return string(buf), nil
}
