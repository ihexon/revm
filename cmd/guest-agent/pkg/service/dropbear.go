package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// DropbearConfig holds dropbear SSH server configuration.
type DropbearConfig struct {
	ListenAddr         string // full "IP:PORT"
	PrivateKeyPath     string
	AuthorizedKeysFile string
	PidFile            string
}

// Dropbear provides dropbear SSH server functionality.
type Dropbear struct {
	cfg DropbearConfig
}

// NewDropbear creates a new Dropbear instance with the given configuration.
func NewDropbear(cfg DropbearConfig) *Dropbear {
	return &Dropbear{cfg: cfg}
}

// GenerateHostKey generates a new dropbear host key.
func (d *Dropbear) GenerateHostKey(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(d.cfg.PrivateKeyPath), 0755); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, DropbearmultiPath(), "dropbearkey", "-t", "ed25519", "-f", d.cfg.PrivateKeyPath)
	cmd.Stderr = StderrWriter()
	cmd.Stdout = StderrWriter()

	logrus.Debugf("dropbearkey: %v", cmd.Args)
	return cmd.Run()
}

// WriteAuthorizedKeys writes the public key to the authorized_keys file.
func (d *Dropbear) WriteAuthorizedKeys(publicKey string) error {
	if err := os.MkdirAll(filepath.Dir(d.cfg.AuthorizedKeysFile), 0755); err != nil {
		return fmt.Errorf("create authorized_keys dir: %w", err)
	}

	if err := os.WriteFile(d.cfg.AuthorizedKeysFile, []byte(publicKey), 0600); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}

	return nil
}

// Start starts the dropbear SSH server. Blocks until the server exits.
func (d *Dropbear) Start(ctx context.Context) error {
	args := []string{
		"dropbear",
		"-D", filepath.Dir(d.cfg.AuthorizedKeysFile),
		"-p", d.cfg.ListenAddr,
		"-r", d.cfg.PrivateKeyPath,
		"-F", // foreground
		"-s", // disable password login
	}

	if d.cfg.PidFile != "" {
		args = append(args, "-P", d.cfg.PidFile)
	}

	cmd := exec.CommandContext(ctx, DropbearmultiPath(), args...)
	cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")
	cmd.Stderr = StderrWriter()
	cmd.Stdout = StderrWriter()

	logrus.Debugf("dropbear: %v", cmd.Args)
	return cmd.Run()
}

// StartGuestSSHServer support TSI/Gvisor network mode
func StartGuestSSHServer(ctx context.Context, vmc *define.Machine) error {
	cfg := DropbearConfig{
		ListenAddr:         vmc.SSHInfo.GuestSSHServerListenAddr,
		PrivateKeyPath:     vmc.SSHInfo.GuestSSHPrivateKeyFile,
		AuthorizedKeysFile: vmc.SSHInfo.GuestSSHAuthorizedKeys,
		PidFile:            vmc.SSHInfo.GuestSSHPidFile,
	}

	dropbear := NewDropbear(cfg)

	if err := dropbear.GenerateHostKey(ctx); err != nil {
		return fmt.Errorf("generate host key: %w", err)
	}

	if err := dropbear.WriteAuthorizedKeys(vmc.SSHInfo.HostSSHPublicKey); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}

	logrus.Infof("SSH server starting on %s", cfg.ListenAddr)
	return dropbear.Start(ctx)
}
