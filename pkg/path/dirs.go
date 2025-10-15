package path

import (
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
)

func GetExecutableDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to eval symlinks: %w", err)
	}

	selfDir := filepath.Dir(path)

	return selfDir, nil
}

func GetBuiltinRootfsPath() (string, error) {
	binDir, err := GetExecutableDir()
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(binDir)

	return filepath.Join(parentDir, define.RootfsDirName), nil
}

func GetLibexecPath() (string, error) {
	binDir, err := GetExecutableDir()
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(binDir)
	return filepath.Join(parentDir, define.LibexecDirName), nil
}
