package system

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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
