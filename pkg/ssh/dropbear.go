//go:build (darwin && arm64) || (linux && (arm64 || amd64))

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
	Provider   string
	Port       int
	KeyFile    string
	RunTimeDir string
	PidFile    string
}

const (
	dropbear    = "dropbear"
	dropbearkey = "dropbearkey"
)

func GetProvider() *SSHServer {
	return &SSHServer{
		Provider:   dropbear,
		Port:       2222,
		KeyFile:    define.DropBearKeyFile,
		RunTimeDir: define.DropBearRuntimeDir,
		PidFile:    define.DropBearPidFile,
	}
}

// CreateSSHKey create ssh keypair for ssh provider
func (s *SSHServer) CreateSSHKey(ctx context.Context) error {
	switch s.Provider {
	case dropbear:
		logrus.Infof("SSH Server provider is dropbear, create ssh key: %q", s.KeyFile)
		if err := os.MkdirAll(filepath.Dir(s.KeyFile), 0755); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, filepath.Join("/", define.PrefixDir3rd, dropbearkey), "-f", s.KeyFile)
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
		cmd := exec.CommandContext(ctx, filepath.Join("/", define.PrefixDir3rd, dropbear), "-r", s.KeyFile, "-D",
			s.RunTimeDir, "-F", "-B", "-P", s.PidFile)
		cmd.Stdin = nil
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")
		return cmd.Run()
	default:
		return errors.New("no ssh server provider found")
	}
}

func StartSSHServer(ctx context.Context) error {
	p := GetProvider()
	if err := p.CreateSSHKey(ctx); err != nil {
		return fmt.Errorf("failed to create ssh key: %w", err)
	}
	if err := p.Start(ctx); err != nil {
		return fmt.Errorf("failed to start ssh server: %w", err)
	}

	return nil
}
