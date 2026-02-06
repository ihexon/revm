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

type XATTRManager interface {
	WriteXATTR(ctx context.Context, filePath string, key string, value string, overwrite bool) error
	GetXATTR(ctx context.Context, filePath string, key string) (string, error)
}

type xattrManager struct {
	bin string
}

func NewXATTRManager() XATTRManager {
	return &xattrManager{
		bin: "/usr/bin/xattr",
	}
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

	cmd := exec.CommandContext(ctx, b.bin, "-w", namespace, value, blkPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Stdin = nil

	return cmd.Run()
}

func (b xattrManager) GetXATTR(ctx context.Context, blkPath string, namespace string) (string, error) {
	blkPath, err := filepath.Abs(filepath.Clean(blkPath))
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("get xattr error with %v: %s", err, errMsg.String())
	}

	return value.String(), nil
}
