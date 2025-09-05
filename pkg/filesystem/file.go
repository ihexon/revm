package filesystem

import (
	"os"

	"github.com/pkg/errors"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	// Some other error (e.g., permission)
	return false, err
}
