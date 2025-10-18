package ssh

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/keygen"
)

var (
	// ErrEmptyDestination is returned when the destination path is empty
	ErrEmptyDestination = errors.New("destination path cannot be empty")
	// ErrKeyGenerationFailed is returned when key generation fails
	ErrKeyGenerationFailed = errors.New("SSH key generation failed")
	// ErrKeyWriteFailed is returned when writing keys to disk fails
	ErrKeyWriteFailed = errors.New("failed to write SSH keys to disk")
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

// GenerateKeyPair generates an SSH key pair and writes it to the specified destination directory.
// The destination path will be converted to an absolute path.
//
// Parameters:
//   - destPath: Directory path where the key pair will be written
//   - opts: Key generation options (use DefaultKeyGenOptions() for defaults)
//
// Returns:
//   - *KeyPair: The generated key pair with metadata
//   - error: Any error encountered during generation or writing
//
// The function will create the destination directory if it doesn't exist.
func GenerateKeyPair(destPath string, opts KeyGenOptions) (*KeyPair, error) {
	if destPath == "" {
		return nil, ErrEmptyDestination
	}

	// Convert to absolute path for consistency
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %q: %w", destPath, err)
	}

	// Ensure the destination directory exists
	if err := ensureDirectory(filepath.Dir(absPath)); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Configure key generation options
	keygenOpts := []keygen.Option{
		keygen.WithKeyType(opts.KeyType),
	}
	if opts.Passphrase != "" {
		keygenOpts = append(keygenOpts, keygen.WithPassphrase(opts.Passphrase))
	}

	// Generate the key pair
	kp, err := keygen.New(absPath, keygenOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyGenerationFailed, err)
	}

	// Write keys to disk
	if err := kp.WriteKeys(); err != nil {
		return nil, fmt.Errorf("%w at %q: %v", ErrKeyWriteFailed, absPath, err)
	}

	return &KeyPair{
		KeyPair:      kp,
		AbsolutePath: absPath,
	}, nil
}

// ensureDirectory creates a directory and all necessary parent directories
func ensureDirectory(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %q", path)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat directory %q: %w", path, err)
	}

	// Create directory with secure permissions
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", path, err)
	}

	return nil
}

// PublicKeyPath returns the path to the public key file
func (kp *KeyPair) PublicKeyPath() string {
	return kp.AbsolutePath + ".pub"
}

// PrivateKeyPath returns the path to the private key file
func (kp *KeyPair) PrivateKeyPath() string {
	return kp.AbsolutePath
}
