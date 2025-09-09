//go:build (darwin && (arm64 || amd64)) || (linux && (arm64 || amd64))

package ssh

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SSHServer struct {
	Provider           string
	Addr               string
	Port               uint64
	KeyFile            string
	RunTimeDir         string
	PidFile            string
	AuthorizedKeysFile string
}

const (
	TypeDropbear = "dropbear"
)

// GetProvider get ssh server provider. A struct of SSHServer contains KeyFile, RunTimeDir, AuthorizedKeysFile, PidFile
// and Provider. all fields are initialed by default.
func GetProvider(cfg SSHServer) *SSHServer {
	sshServer := &SSHServer{
		Provider: cfg.Provider,
		// the port will be assigned from vmc.SSHInfo.Port, which is randomly assigned.
		Port:               cfg.Port,
		Addr:               cfg.Addr,
		KeyFile:            define.DropBearKeyFile,
		RunTimeDir:         define.DropBearRuntimeDir,
		AuthorizedKeysFile: filepath.Join(define.DropBearRuntimeDir, "authorized_keys"),
		PidFile:            define.DropBearPidFile,
	}
	return sshServer
}

func (s *SSHServer) GenerateSSHKeyFile(ctx context.Context) error {
	switch s.Provider {
	case TypeDropbear:
		logrus.Infof("generate sshkey pair for guest: %q", s.KeyFile)
		if err := os.MkdirAll(filepath.Dir(s.KeyFile), 0755); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, "dropbearkey", "-f", s.KeyFile)
		cmd.Stdin = nil
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stderr
		}
		logrus.Debugf("dropbearkey cmdline: %q", cmd.Args)
		return cmd.Run()
	default:
		return errors.New("no ssh server provider found")
	}
}

func (s *SSHServer) Start(ctx context.Context) error {
	switch s.Provider {
	case TypeDropbear:
		if s.Port == 0 {
			return errors.New("ssh port is not set")
		}

		logrus.Infof("start guest built-in server in %s:%d", s.Addr, s.Port)
		cmd := exec.CommandContext(ctx, "dropbear", "-p", strconv.Itoa(int(s.Port)), "-r", s.KeyFile, "-D",
			s.RunTimeDir, "-F", "-B", "-P", s.PidFile)
		cmd.Stdin = nil
		cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stderr
		}

		logrus.Debugf("dropbear cmdline: %q", cmd.Args)
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
		if err := f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	logrus.Infof("write AuthorizedKeys from vmconfig.json to %q", s.AuthorizedKeysFile)
	logrus.Debugf("authorizedKeys content: %q", vmc.SSHInfo.HostSSHPublicKey)
	_, err = f.WriteString(vmc.SSHInfo.HostSSHPublicKey)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func StartSSHServer(ctx context.Context, cfg SSHServer) error {
	p := GetProvider(cfg)
	if err := p.GenerateSSHKeyFile(ctx); err != nil {
		return fmt.Errorf("failed to create ssh key: %w", err)
	}

	if err := p.WriteAuthorizedkeysFile(); err != nil {
		return err
	}

	return p.Start(ctx)
}
