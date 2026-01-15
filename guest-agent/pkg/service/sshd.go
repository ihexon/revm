package service

import (
	"context"
	_ "embed"
	"fmt"

	"os"
	"os/exec"
	"path/filepath"

	"guestAgent/pkg/pathutils"
	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

//go:embed dropbearmulti
var dropbearmulti []byte

type SSHServer struct {
	dropbearMulti string
	cfg           *cfg
}

type cfg struct {
	addr               string
	port               uint64
	keyPairFiles       string
	runTimeDir         string
	pidFile            string
	authorizedKeysFile string
}

func StartGuestSSHServer(ctx context.Context, vmc *define.VMConfig) error {
	sshServer := NewBuiltinSSHServer(
		// the dropbear binary which used to run ssh server
		vmc.ExternalTools.LinuxTools.DropBear,
		// the ssh server run config
		&cfg{
			port:               define.DefaultGuestSSHDPort,
			addr:               define.UnspecifiedAddress,
			keyPairFiles:       define.DropBearKeyFile,
			runTimeDir:         define.DropBearRuntimeDir,
			authorizedKeysFile: filepath.Join(define.DropBearRuntimeDir, "authorized_keys"),
			pidFile:            define.DropBearPidFile,
		},
	)

	if err := sshServer.getBinaries(); err != nil {
		return err
	}

	if err := sshServer.GenerateSSHKeyFile(ctx); err != nil {
		return fmt.Errorf("failed to create ssh key: %w", err)
	}

	if err := sshServer.WriteAuthorizedkeysFile(ctx, vmc); err != nil {
		return err
	}

	errChan := make(chan error, 1)

	go func() {
		errChan <- sshServer.Start(ctx)
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}

func NewBuiltinSSHServer(dropbearBinPath string, cfg *cfg) *SSHServer {
	return &SSHServer{
		dropbearMulti: dropbearBinPath,
		cfg:           cfg,
	}
}

func getBinary(filePath string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	if pathutils.IsPathExist(filePath) {
		return nil
	}

	return os.WriteFile(filePath, b, 0755)
}

func (s *SSHServer) getBinaries() error {
	if err := getBinary(s.dropbearMulti, dropbearmulti); err != nil {
		return err
	}
	unix.Sync()
	return os.Chmod(s.dropbearMulti, 0755)
}

func (s *SSHServer) GenerateSSHKeyFile(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.keyPairFiles), 0755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, s.dropbearMulti, "dropbearkey", "-f", s.cfg.keyPairFiles)

	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	logrus.Debugf("dropbearkey cmdline: %q", cmd.Args)
	return cmd.Run()
}

// WriteAuthorizedkeysFile write host public key (from vmconfig.HostSSHPublicKey) to dropbear's authorized_keys.
func (s *SSHServer) WriteAuthorizedkeysFile(ctx context.Context, vmc *define.VMConfig) error {
	f, err := os.Create(s.cfg.authorizedKeysFile)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer func(f *os.File) {
		if err := f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	_, err = f.WriteString(vmc.SSHInfo.HostSSHPublicKey)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (s *SSHServer) Start(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, s.dropbearMulti, "dropbear", "-p", fmt.Sprintf("%s:%d", s.cfg.addr, s.cfg.port), "-r", s.cfg.keyPairFiles, "-D",
		s.cfg.runTimeDir, "-F", "-s", "-E", "-P", s.cfg.pidFile)

	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "PASS_FILEPEM_CHECK=1")

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	logrus.Debugf("start ssh server cmdline: %q", cmd.Args)
	return cmd.Run()
}
