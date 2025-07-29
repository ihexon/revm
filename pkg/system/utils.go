package system

import (
	"fmt"
	"os"
	"path/filepath"
)

func Get3rdDir() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to eval symlinks: %w", err)
	}

	path = filepath.Join(filepath.Dir(filepath.Dir(path)), "3rd")
	return path, nil
}
