package ssh_v2

import (
	"github.com/charmbracelet/keygen"
)

func GenerateKey() (privateKey, publicKey []byte, err error) {
	kp, err := keygen.New("", keygen.WithKeyType(keygen.Ed25519))
	if err != nil {
		return nil, nil, err
	}
	return kp.RawPrivateKey(), kp.RawAuthorizedKey(), nil
}

// GenerateKeyWithPassphrase generates an encrypted SSH key pair.
func GenerateKeyWithPassphrase(path, passphrase string) (privateKey, publicKey string, err error) {
	kp, err := keygen.New(path,
		keygen.WithKeyType(keygen.Ed25519),
		keygen.WithPassphrase(passphrase),
	)
	if err != nil {
		return "", "", err
	}
	return path, path + ".pub", kp.WriteKeys()
}
