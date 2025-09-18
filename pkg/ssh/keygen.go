package ssh

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/keygen"
)

// GenerateHostSSHKeyPair generates an SSH key pair for the host and writes it to the specified destination directory.
func GenerateHostSSHKeyPair(dest string) (*keygen.KeyPair, error) {
	if dest == "" {
		return nil, fmt.Errorf("function parameter error, dest is empty")
	}

	dest, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}

	k, err := keygen.New(
		dest,
		keygen.WithKeyType(keygen.Ed25519),
	)

	if err != nil {
		return nil, fmt.Errorf("keygen.New error: %w", err)
	}

	return k, k.WriteKeys()
}
