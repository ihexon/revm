//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package system

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func Get3rdDir() (string, error) {
	path := os.Getenv("REVM_3RD_DIR")
	if path != "" {
		logrus.Warnf("env REVM_3RD_DIR set %q, use it instead of default", path)
		return path, nil
	}

	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to eval symlinks: %w", err)
	}

	selfDir := filepath.Dir(path)
	parentDir := filepath.Dir(selfDir)

	path = filepath.Join(parentDir, define.ThirdPartDirPrefix)
	return path, nil
}

func GenerateRandomID() string {
	u := uuid.NewString()

	// Hash the UUID
	h := sha256.Sum256([]byte(u))

	// Encode hash in URL-safe Base64 (shorter than hex)
	encoded := base64.RawURLEncoding.EncodeToString(h[:])

	// Take first 6 chars
	return encoded[:6]
}

func Get3rdUtilsPath(name string) (string, error) {
	dir, err := Get3rdDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, runtime.GOOS, "bin", name), nil
}

func Get3rdUtilsPathForLinux(name string) (string, error) {
	dir, err := Get3rdDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "linux", "bin", name), nil
}

func GetGuestLinuxUtilsBinPath(name string) string {
	return filepath.Join(define.GuestLinuxUtilsBinDir, name)
}
