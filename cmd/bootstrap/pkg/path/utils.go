package path

import (
	"errors"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func GetGuestLinuxUtilsBinPath(name string) string {
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
