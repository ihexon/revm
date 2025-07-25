package system

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
)

func CopyBootstrapTo(rootfs string) error {
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("failed to eval symlinks: %w", err)
	}

	path = filepath.Join(filepath.Dir(path), define.Bootstrap)
	logrus.Infof("bootstrap path %q", path)

	return CopyFile(path, filepath.Join(rootfs, define.Bootstrap))
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

	srcFd, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer srcFd.Close()
	srcInfo, err := srcFd.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	dstFd, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer dstFd.Close()

	logrus.Infof("copy file from %q to %q", src, dst)
	written, err := io.Copy(dstFd, srcFd)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	logrus.Infof("copied %d bytes", written)

	return nil
}
