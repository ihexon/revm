package pathutils

import (
	"errors"
	"os"

	"github.com/sirupsen/logrus"
)

func IsPathExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false
	}

	if err != nil {
		logrus.Warnf("os.Stat %q error: %v", path, err)
		return false
	}

	return true
}
