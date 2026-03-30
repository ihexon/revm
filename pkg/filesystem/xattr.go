//go:build darwin && (arm64 || amd64)

package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type XattrManager interface {
	SetXattr(ctx context.Context, filePath string, key string, value string, overwrite bool) error
	GetXattr(ctx context.Context, filePath string, key string) (string, error)
	LookupXattr(ctx context.Context, filePath string, key string) (string, bool, error)
}

type xattrManager struct {
	bin string
}

func NewXattrManager() XattrManager {
	return &xattrManager{
		bin: "/usr/bin/xattr",
	}
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

	existValue, ok, _ := b.LookupXattr(ctx, blkPath, namespace)
	if ok && existValue != "" && !overwrite {
		return nil
	}

	cmd := exec.CommandContext(ctx, b.bin, "-w", namespace, value, blkPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Stdin = nil

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setxattr %q on %q: %w", namespace, blkPath, err)
	}
	return nil
}

func (b xattrManager) GetXattr(ctx context.Context, blkPath string, namespace string) (string, error) {
	value, ok, err := b.LookupXattr(ctx, blkPath, namespace)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("getxattr %q on %q: attribute not found", namespace, blkPath)
	}

	return value, nil
}

func (b xattrManager) LookupXattr(ctx context.Context, blkPath string, namespace string) (string, bool, error) {
	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return "", false, err
	}

	cmd := exec.CommandContext(ctx, b.bin, "-p", namespace, blkPath)
	var (
		value  bytes.Buffer
		errMsg bytes.Buffer
	)
	cmd.Stderr = &errMsg
	cmd.Stdout = &value
	cmd.Stdin = nil

	if err = cmd.Run(); err != nil {
		msg := strings.TrimSpace(errMsg.String())
		if strings.Contains(msg, "No such xattr:") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("getxattr %q on %q: %w (%s)", namespace, blkPath, err, msg)
	}

	return value.String(), true, nil
}
