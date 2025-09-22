//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package system

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func getExecutableDir() (string, error) {
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

func GenerateRandomID() string {
	u := uuid.NewString()

	// Hash the UUID
	h := sha256.Sum256([]byte(u))

	// Encode hash in URL-safe Base64 (shorter than hex)
	encoded := base64.RawURLEncoding.EncodeToString(h[:])

	// Take first 6 chars
	return encoded[:6]
}

func BootstrapPathInGuestView() string {
	return filepath.Join("/", define.BoostrapFileName)
}

func GetBuiltinRootfsPath() (string, error) {
	binDir, err := getExecutableDir()
	if err != nil {
		return "", fmt.Errorf("failed to get executable dir: %w", err)
	}
	parentDir := filepath.Dir(binDir)

	rootfsDir := filepath.Join(parentDir, define.RootfsDirName)
	return rootfsDir, nil
}

func GetLibexecNamePath(name string) (string, error) {
	binDir, err := getExecutableDir()
	if err != nil {
		return "", fmt.Errorf("failed to get executable dir: %w", err)
	}
	parentDir := filepath.Dir(binDir)
	libexecDirPath := filepath.Join(parentDir, define.LibexecDirName)
	return filepath.Join(libexecDirPath, name), nil
}
