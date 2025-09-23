//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package system

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/google/uuid"
)

func GenerateRandomID() string {
	u := uuid.NewString()

	// Hash the UUID
	h := sha256.Sum256([]byte(u))

	// Encode hash in URL-safe Base64 (shorter than hex)
	encoded := base64.RawURLEncoding.EncodeToString(h[:])

	// Take first 6 chars
	return encoded[:6]
}
