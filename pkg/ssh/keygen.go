package ssh

import (
	"github.com/charmbracelet/keygen"
	"github.com/sirupsen/logrus"
)

// KeyPair represents an SSH key pair with its metadata
type KeyPair struct {
	*keygen.KeyPair
	AbsolutePath string
}

// KeyGenOptions configures SSH key generation
type KeyGenOptions struct {
	KeyType    keygen.KeyType
	Passphrase string
}

// DefaultKeyGenOptions returns sensible defaults for key generation
func DefaultKeyGenOptions() KeyGenOptions {
	return KeyGenOptions{
		KeyType: keygen.Ed25519,
	}
}

func GenerateKeyPair(keyFile string, opts KeyGenOptions) (*KeyPair, error) {
	logrus.Debugf("generating SSH key pair (type: %v) at %q", opts.KeyType, keyFile)

	keygenOpts := []keygen.Option{
		keygen.WithKeyType(opts.KeyType),
	}
	if opts.Passphrase != "" {
		keygenOpts = append(keygenOpts, keygen.WithPassphrase(opts.Passphrase))
	}

	// Generate the key pair
	kp, err := keygen.New(keyFile, keygenOpts...)
	if err != nil {
		logrus.Errorf("failed to generate SSH key pair: %v", err)
		return nil, err
	}

	return &KeyPair{
		KeyPair:      kp,
		AbsolutePath: keyFile,
	}, nil
}

// PublicKeyPath returns the path to the public key file
func (kp *KeyPair) PublicKeyPath() string {
	return kp.AbsolutePath + ".pub"
}

// PrivateKeyPath returns the path to the private key file
func (kp *KeyPair) PrivateKeyPath() string {
	return kp.AbsolutePath
}
