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
	ListenAddr         string
	ListenPort         uint16
	PrivateKeyPath     string
	AuthorizedKeysFile string
	PidFile            string
}

// Dropbear provides dropbear SSH server functionality.
type Dropbear struct {
	path string
	cfg  DropbearConfig
}

// NewDropbear creates a new Dropbear instance with the given configuration.
func NewDropbear(cfg DropbearConfig) (*Dropbear, error) {
	path, err := DropbearmultiBinary.ExtractToDir(define.GuestHiddenBinDir)
	if err != nil {
		return nil, err
	}

	return &Dropbear{path: path, cfg: cfg}, nil
}

// GenerateHostKey generates a new dropbear host key.
func (d *Dropbear) GenerateHostKey(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(d.cfg.PrivateKeyPath), 0755); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, d.path, "dropbearkey", "-t", "ed25519", "-f", d.cfg.PrivateKeyPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

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
		"-p", fmt.Sprintf("%s:%d", d.cfg.ListenAddr, d.cfg.ListenPort),
		"-r", d.cfg.PrivateKeyPath,
		"-F", // foreground
		"-s", // disable password login
		"-E", // log to stderr
	}

	if d.cfg.PidFile != "" {
		args = append(args, "-P", d.cfg.PidFile)
	}

	cmd := exec.CommandContext(ctx, d.path, args...)
	cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	logrus.Debugf("dropbear: %v", cmd.Args)
	return cmd.Run()
}

// StartGuestSSHServer starts the guest SSH server with default configuration.
func StartGuestSSHServer(ctx context.Context, vmc *define.VMConfig) error {
	cfg := DropbearConfig{
		ListenAddr:         define.UnspecifiedAddress,
		ListenPort:         define.GuestSSHServerPort,
		PrivateKeyPath:     define.DropBearPrivateKeyPath,
		AuthorizedKeysFile: filepath.Join(define.DropBearRuntimeDir, "authorized_keys"),
		PidFile:            define.DropBearPidFile,
	}

	dropbear, err := NewDropbear(cfg)
	if err != nil {
		return fmt.Errorf("create dropbear: %w", err)
	}

	if err := dropbear.GenerateHostKey(ctx); err != nil {
		return fmt.Errorf("generate host key: %w", err)
	}

	if err := dropbear.WriteAuthorizedKeys(vmc.SSHInfo.HostSSHPublicKey); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}

	logrus.Infof("SSH server starting on port %d", cfg.ListenPort)
	return dropbear.Start(ctx)
}
