package ssh

import (
	"github.com/charmbracelet/keygen"
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
	keygenOpts := []keygen.Option{
		keygen.WithKeyType(opts.KeyType),
	}
	if opts.Passphrase != "" {
		keygenOpts = append(keygenOpts, keygen.WithPassphrase(opts.Passphrase))
	}

	// Generate the key pair
	kp, err := keygen.New(keyFile, keygenOpts...)
	if err != nil {
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
