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

// GetDarwinToolsPath returns the path to the darwin tools
func GetDarwinToolsPath(name string) (string, error) {
	path, err := GetLibexecPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(path, name), err
}

// GetBinNamePath Resolve the absolute path of another program in the same directory as revm
//
// for example, if the revm location is "/usr/bin/revm", and the program name is "myproj",
// the absolute path will be "/usr/bin/myproj"
func GetBinNamePath(name string) (string, error) {
	dir, err := GetExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

type view int

const (
	HostView view = iota
	GuestView
)

// GetToolsPath3rd given a program name, return the absolute path of the program in the 3rd rootfs.
//
//	for example, if the program name is "myproj":
//		the absolute path will be "$ROOTFS/3rd/bin/myproj" in host view
//		the absolute path will be "/3rd/usr/bin/myproj" in guest view.
func GetToolsPath3rd(name string, view view) (string, error) {
	if view == GuestView {
		return filepath.Join(define.GuestLinuxUtilsBinDir, name), nil
	}

	rootfsPath, err := GetBuiltinRootfsPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(rootfsPath, define.GuestLinuxUtilsBinDir, name), nil
}
