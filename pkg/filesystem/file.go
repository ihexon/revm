package filesystem

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	logrus.Debugf("os.Stat error: %v", err)
	return false, err
}
