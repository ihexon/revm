package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func Copy3rdFileTo(rootfs string) error {
	path, err := Get3rdDir()
	if err != nil {
		return fmt.Errorf("failed to get 3rd dir: %w", err)
	}

	fileList := []string{
		"bootstrap",
		"busybox.static",
		"dropbear",
		"dropbearkey",
	}

	for _, file := range fileList {
		src := filepath.Join(path, "linux", "bin", file)
		dst := filepath.Join(rootfs, "3rd", file)
		logrus.Debugf("copy file from %q to %q", src, dst)
		if err := CopyFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	return nil
}

func CopyFile(src, dst string) error {
	src, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	dst, err = filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create dir: %w", err)
	}

	srcFd, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func(srcFd *os.File) {
		if err := srcFd.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(srcFd)

	srcInfo, err := srcFd.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	dstFd, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer dstFd.Close()

	_, err = io.Copy(dstFd, srcFd)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}
