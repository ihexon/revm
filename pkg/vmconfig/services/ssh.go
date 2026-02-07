//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package services

import (
	"context"
	"linuxvm/pkg/define"
	sshv2 "linuxvm/pkg/ssh_v2"
	"linuxvm/pkg/vmconfig/internal"
	"os"
	"path/filepath"
)

// SSHConfigurator handles SSH key generation and configuration.
type SSHConfigurator struct {
	pathMgr *internal.PathManager
}

// NewSSHConfigurator creates a new SSH configurator.
func NewSSHConfigurator(pathMgr *internal.PathManager) *SSHConfigurator {
	return &SSHConfigurator{pathMgr: pathMgr}
}

// Configure generates SSH keys and sets up SSH configuration.
func (s *SSHConfigurator) Configure(ctx context.Context, vmc *define.VMConfig) error {
	keyPath := s.pathMgr.GetSSHPrivateKeyFile()
	pubKeyPath := keyPath + ".pub"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return err
	}

	privateKey, publicKey, err := sshv2.GenerateKey()
	if err != nil {
		return err
	}
	if err = os.WriteFile(keyPath, privateKey, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(pubKeyPath, publicKey, 0644); err != nil {
		return err
	}

	vmc.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      string(publicKey),
		HostSSHPrivateKey:     string(privateKey),
		HostSSHPrivateKeyFile: keyPath,
	}

	return nil
}
