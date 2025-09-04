package ssh

import (
	"fmt"

	"github.com/charmbracelet/keygen"
	"github.com/pkg/errors"
)

// GenerateHostSSHKeyPair generates an SSH key pair for the host and writes it to the specified destination directory.
func GenerateHostSSHKeyPair(dest string) (*keygen.KeyPair, error) {
	if dest == "" {
		return nil, errors.New("failed to generate SSH key pair, ssh key pair dest is empty")
	}

	k, err := keygen.New(
		dest,
		keygen.WithKeyType(keygen.Ed25519),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to generate ssh keypair for host: %w", err)
	}

	if err = k.WriteKeys(); err != nil {
		return nil, fmt.Errorf("failed to write ssh keypair for host: %w", err)
	}

	return k, nil
}
