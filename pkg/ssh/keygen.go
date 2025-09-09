package ssh

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/keygen"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GenerateHostSSHKeyPair generates an SSH key pair for the host and writes it to the specified destination directory.
func GenerateHostSSHKeyPair(dest string) (*keygen.KeyPair, error) {
	if dest == "" {
		return nil, errors.New("function parameter error, dest is empty")
	}

	dest, err := filepath.Abs(dest)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	logrus.Infof("ssh keypair will be generate in %q with keytype %q", dest, keygen.Ed25519)
	k, err := keygen.New(
		dest,
		keygen.WithKeyType(keygen.Ed25519),
	)

	if err != nil {
		return nil, fmt.Errorf("new generates a KeyPair with error : %w", err)
	}

	return k, k.WriteKeys()
}
