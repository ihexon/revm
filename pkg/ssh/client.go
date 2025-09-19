//  SPDX-FileCopyrightText: 2024-2025 OOMOL, Inc. <https://www.oomol.com>
//  SPDX-License-Identifier: MPL-2.0

package ssh

import (
	"context"
	"errors"
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

type RunConfig struct {
	SSHClient *ssh.Client
	// SSH session.
	MySession *ssh.Session
	// Signal send when the context is canceled
	Signal ssh.Signal

	Pty bool

	StdoutWriterFn func() error
	StderrWriterFn func() error

	// Path to command executable filename
	Bin string
	// Command args.
	Args []string

	Addr string
	Port uint64
	User string
	Key  string

	// callback
	CleanUp system.CleanupCallback
}

// SetStopSignal sets the signal to send when the context is canceled.
func (c *RunConfig) SetStopSignal(signal ssh.Signal) {
	c.Signal = signal
}

// SetCmdLine sets the command line to execute.
func (c *RunConfig) SetCmdLine(name string, args []string) {
	c.Bin = name
	c.Args = args
}

func (c *RunConfig) SetPty(pty bool) {
	c.Pty = pty
}

func (c *RunConfig) IsPty() bool {
	return c.Pty
}

// CmdString returns the command line string, with each parameter wrapped in ""
func (c *RunConfig) CmdString() string {
	args := append([]string{c.Bin}, c.Args...)
	for i, s := range args {
		args[i] = fmt.Sprintf("\"%s\"", s)
	}
	return strings.Join(args, " ")
}

func (c *RunConfig) Run(ctx context.Context) error {
	context.AfterFunc(ctx, func() {
		if c.MySession != nil {
			logrus.Debugf("send signal %q to %q, cause by %v", c.Signal, c.Bin, context.Cause(ctx))
			if err := c.MySession.Signal(c.Signal); err != nil {
				logrus.Debugf("send signal %q to %q failed: %v", c.Signal, c.Bin, err)
			}
		}
	})

	var wg sync.WaitGroup

	if err := c.MySession.Start(c.CmdString()); err != nil {
		return fmt.Errorf("failed to run shell: %w", err)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = c.StdoutWriterFn()
		logrus.Debugf("StdoutWriterFn done")
	}()

	go func() {
		defer wg.Done()
		_ = c.StderrWriterFn()
		logrus.Debugf("StderrWriterFn done")
	}()

	wg.Wait()
	logrus.Debugf("StderrWriterFn/StdoutWriterFn group done")

	return c.MySession.Wait()
}

func (c *RunConfig) Connect(ctx context.Context, gvCtl string) error {
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
		return errors.New("failed to parse private key")
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

func (c *RunConfig) RequestPTY(ctx context.Context) (func(), error) {
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

func (c *RunConfig) WriteOutputTo(stdoutWriter, stderrWriter io.Writer) error {
	outPipeReader, err := c.MySession.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	errPipeReader, err := c.MySession.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	c.StdoutWriterFn = func() error {
		_, _ = io.Copy(stdoutWriter, outPipeReader)
		return nil
	}

	c.StderrWriterFn = func() error {
		_, _ = io.Copy(stderrWriter, errPipeReader)
		return nil
	}

	c.MySession.Stdin = nil

	return nil
}

func NewCfg(addr, user string, port uint64, keyFile string) *RunConfig {
	return &RunConfig{
		Addr:    addr,
		User:    user,
		Port:    port,
		Key:     keyFile,
		Signal:  ssh.SIGKILL,
		CleanUp: system.CleanUp(),
	}
}

func StartKeepAlive(ctx context.Context, conn *ssh.Client) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("stop sending keepalive signal to ssh")
			return
		case <-ticker.C:
			if _, _, err := conn.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				logrus.Debugf("failed to send keepalive: %v", err)
			}
			continue
		}
	}
}
