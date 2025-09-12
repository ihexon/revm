//  SPDX-FileCopyrightText: 2024-2025 OOMOL, Inc. <https://www.oomol.com>
//  SPDX-License-Identifier: MPL-2.0

package ssh

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/system"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type SSHRunConfig struct {
	SSHClient *ssh.Client
	// SSH session.
	MySession *ssh.Session
	// Signal send when the context is canceled
	Signal ssh.Signal
	Stdout io.Reader
	Stderr io.Reader

	// Path to command executable filename
	Bin string
	// Command args.
	Args []string

	Pty bool

	Addr string
	Port uint64
	User string
	Key  string

	// callback
	CleanUp   system.CleanupCallback
	OtherFunc []func() error
}

func (c *SSHRunConfig) readSessionOutputBlock() {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(os.Stdout, c.Stdout); err != nil {
			logrus.Errorf("failed to copy stdout: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if _, err := io.Copy(os.Stderr, c.Stderr); err != nil {
			logrus.Errorf("failed to copy stderr: %v", err)
		}
	}()

	wg.Wait()
}

// SetStopSignal sets the signal to send when the context is canceled.
func (c *SSHRunConfig) SetStopSignal(signal ssh.Signal) {
	c.Signal = signal
}

// SetCmdLine sets the command line to execute.
func (c *SSHRunConfig) SetCmdLine(name string, args []string) {
	c.Bin = name
	c.Args = args
}

func (c *SSHRunConfig) SetPty(pty bool) {
	c.Pty = pty
}

func (c *SSHRunConfig) IsPty() bool {
	return c.Pty
}

// CmdString returns the command line string, with each parameter wrapped in ""
func (c *SSHRunConfig) CmdString() string {
	args := append([]string{c.Bin}, c.Args...)
	for i, s := range args {
		args[i] = fmt.Sprintf("\"%s\"", s)
	}
	return strings.Join(args, " ")
}

func (c *SSHRunConfig) Run(ctx context.Context) error {
	context.AfterFunc(ctx, func() {
		if c.MySession != nil {
			logrus.Warnf("send signal %q to %q, cause by %v", c.Signal, c.Bin, context.Cause(ctx))
			if err := c.MySession.Signal(c.Signal); err != nil {
				logrus.Warnf("send signal %q to %q failed: %v", c.Signal, c.Bin, err)
			}
		}
	})

	if err := c.MySession.Start(c.CmdString()); err != nil {
		return fmt.Errorf("failed to run shell: %w", err)
	}

	c.readSessionOutputBlock()

	return c.MySession.Wait()
}

func (c *SSHRunConfig) Connect(ctx context.Context, gvCtl string) error {
	gvpConn, err := net.DialTimeout("unix", gvCtl, 2*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to gvproxy control endpoint: %w", err)
	}

	c.CleanUp.Add(func() error {
		_ = gvpConn.Close()
		return nil
	})

	if err = transport.Tunnel(gvpConn, c.Addr, int(c.Port)); err != nil {
		return err
	}

	f, err := os.ReadFile(c.Key)
	if err != nil {
		return fmt.Errorf("read ssh key failed: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(f)
	if err != nil {
		return fmt.Errorf("failed to parse private key")
	}

	conn, chans, reqs, err := ssh.NewClientConn(gvpConn, "", &ssh.ClientConfig{
		User:            c.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})

	if err != nil {
		return fmt.Errorf("failed to create ssh client connection: %w", err)
	}

	c.SSHClient = ssh.NewClient(conn, chans, reqs)
	c.CleanUp.Add(func() error {
		return c.SSHClient.Close()
	})

	c.MySession, err = c.SSHClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}

	c.CleanUp.Add(func() error {
		_ = c.MySession.Close()
		return nil
	})

	return nil
}

func (c *SSHRunConfig) RequestPTY(ctx context.Context) (func(), error) {
	resetFunc := func() {}

	if system.IsTerminal() {
		state, err := system.MakeStdinRaw()
		if err != nil {
			return nil, err
		}

		system.OnTerminalResize(ctx, func(width, height int) { _ = c.MySession.WindowChange(height, width) })
		if err = c.MySession.RequestPty(system.GetTerminalType(), 80, 80, ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.IUTF8:         1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}); err != nil {
			return nil, fmt.Errorf("failed to request pty: %w", err)
		}
		c.MySession.Stdin = os.Stdin
		resetFunc = func() {
			system.ResetStdin(state)
		}
	}

	return resetFunc, nil
}

func (c *SSHRunConfig) MakeStdPipe() error {
	outPipe, err := c.MySession.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	errPipe, err := c.MySession.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	c.Stdout = outPipe
	c.Stderr = errPipe
	c.MySession.Stdin = nil

	return nil
}

func NewCfg(addr, user string, port uint64, keyFile string) *SSHRunConfig {
	return &SSHRunConfig{
		Addr:      addr,
		User:      user,
		Port:      port,
		Key:       keyFile,
		Signal:    ssh.SIGKILL,
		CleanUp:   system.CleanUp(),
		OtherFunc: make([]func() error, 1),
	}
}
