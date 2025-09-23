package path

import (
	"errors"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// GetToolsPath3rd given a program name, return the absolute path of the program in the 3rd rootfs.
//
//	for example, if the program name is "myproj":
//		the absolute path will be "/3rd/usr/bin/myproj" in guest view.
func GetToolsPath3rd(name string) string {
	return filepath.Join(define.GuestLinuxUtilsBinDir, name)
}

func IsPathExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false
	}

	if err != nil {
		logrus.Debugf("os.Stat %q error: %v", path, err)
		return false
	}

	return true
}
