//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package system

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
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

func GenerateRandomID() string {
	u := uuid.NewString()

	// Hash the UUID
	h := sha256.Sum256([]byte(u))

	// Encode hash in URL-safe Base64 (shorter than hex)
	encoded := base64.RawURLEncoding.EncodeToString(h[:])

	// Take first 6 chars
	return encoded[:6]
}
