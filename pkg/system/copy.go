package system

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
)

func CopyBootstrapInToRootFS(rootfs string) error {
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("failed to eval symlinks: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	path = filepath.Join(filepath.Dir(path), "bootstrap-arm64")
	logrus.Infof("dhclient4 client path %q", path)

	fd, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer fd.Close()

	destPath := filepath.Join(rootfs, "bootstrap-arm64")

	destFd, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer destFd.Close()

	logrus.Infof("copy file from %q to %q", path, destPath)
	_, err = io.Copy(destFd, fd)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	logrus.Infof("chmod file %q to 0755", destPath)
	if err = os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("failed to chmod file: %w", err)
	}

	return nil
}
