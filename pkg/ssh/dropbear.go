//go:build (darwin && (arm64 || amd64)) || (linux && (arm64 || amd64))

package ssh

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SSHServer struct {
	Provider           string
	Port               int
	KeyFile            string
	RunTimeDir         string
	PidFile            string
	AuthorizedKeysFile string
}

const (
	dropbear    = "dropbear"
	dropbearkey = "dropbearkey"
)

// GetProvider get ssh server provider. A struct of SSHServer contains KeyFile, RunTimeDir, AuthorizedKeysFile, PidFile
// and Provider. all fields are initialed by default.
func GetProvider() *SSHServer {
	return &SSHServer{
		Provider:           dropbear,
		Port:               2222,
		KeyFile:            define.DropBearKeyFile,
		RunTimeDir:         define.DropBearRuntimeDir,
		AuthorizedKeysFile: filepath.Join(define.DropBearRuntimeDir, "authorized_keys"),
		PidFile:            define.DropBearPidFile,
	}
}

func (s *SSHServer) GenerateSSHKeyFile(ctx context.Context) error {
	switch s.Provider {
	case dropbear:
		logrus.Infof("generate sshkey file: %q", s.KeyFile)
		if err := os.MkdirAll(filepath.Dir(s.KeyFile), 0755); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, dropbearkey, "-f", s.KeyFile)
		cmd.Stdin = nil
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		return cmd.Run()
	default:
		return errors.New("no ssh server provider found")
	}
}

func (s *SSHServer) Start(ctx context.Context) error {
	switch s.Provider {
	case dropbear:
		logrus.Infof("SSH Server provider is dropbear, start ssh server")
		cmd := exec.CommandContext(ctx, dropbear, "-r", s.KeyFile, "-D",
			s.RunTimeDir, "-F", "-B", "-P", s.PidFile)
		cmd.Stdin = nil
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")
		logrus.Infof("dropbear cmdline: %q", cmd.Args)
		return cmd.Run()
	default:
		return errors.New("no ssh server provider found")
	}
}

// WriteAuthorizedkeysFile write host public key (from vmconfig.HostSSHPublicKey) to dropbear's authorized_keys.
func (s *SSHServer) WriteAuthorizedkeysFile() error {
	vmc, err := define.LoadVMCFromFile(filepath.Join("/", define.VMConfigFile))
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	f, err := os.Create(s.AuthorizedKeysFile)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	logrus.Infof("write host public key to %q", s.AuthorizedKeysFile)
	_, err = f.WriteString(vmc.HostSSHPublicKey)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func StartSSHServer(ctx context.Context) error {
	p := GetProvider()
	if err := p.GenerateSSHKeyFile(ctx); err != nil {
		return fmt.Errorf("failed to create ssh key: %w", err)
	}

	if err := p.WriteAuthorizedkeysFile(); err != nil {
		return err
	}

	return p.Start(ctx)
}
