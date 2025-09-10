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
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Addr string
	Port uint64
	User string
	Auth []ssh.AuthMethod
}

type client struct {
	// Path to command executable filename
	name string
	// Command args.
	args []string
	// SSH session.
	mySession *ssh.Session
	// Signal send when the context is canceled
	signal ssh.Signal
	// ssh client configure
	config *Config
}

// SetStopSignal sets the signal to send when the context is canceled.
func (c *client) SetStopSignal(signal ssh.Signal) {
	c.signal = signal
}

// SetCmdLine sets the command line to execute.
func (c *client) setCmdLine(name string, args []string) {
	c.name = name
	c.args = args
}

// String returns the command line string, with each parameter wrapped in ""
func (c *client) String() string {
	args := append([]string{c.name}, c.args...)
	for i, s := range args {
		args[i] = fmt.Sprintf("\"%s\"", s)
	}
	return strings.Join(args, " ")
}

func (c *client) RunOverGVProxyVSock(ctx context.Context, gvCtl string) error {
	gvpConn, err := net.DialTimeout("unix", gvCtl, 2*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to gvproxy control endpoint: %w", err)
	}

	if err = transport.Tunnel(gvpConn, c.config.Addr, int(c.config.Port)); err != nil {
		return err
	}

	conn, chans, reqs, err := ssh.NewClientConn(gvpConn, "", &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            c.config.Auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		if err = gvpConn.Close(); err != nil {
			logrus.Errorf("ssh over vsock failed to close gvpConn: %v", err)
		}
		return fmt.Errorf("failed to create ssh client connection: %w", err)
	}

	sshclient := ssh.NewClient(conn, chans, reqs)
	defer func() {
		if err := sshclient.Close(); err != nil {
			logrus.Errorf("failed to close ssh client: %v", err)
		}
	}()

	c.mySession, err = sshclient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer func() {
		logrus.Debugf("ssh over vsock done, close ssh session")
		if err := c.mySession.Close(); err != nil && err != io.EOF {
			logrus.Errorf("failed to close ssh session: %v", err)
		}
		c.mySession = nil
	}()

	if system.IsTerminal() {
		state, err := system.MakeStdinRaw()
		if err != nil {
			return err
		}
		defer system.ResetStdin(state)

		system.OnTerminalResize(ctx, func(width, height int) { _ = c.mySession.WindowChange(height, width) })
		if err = c.mySession.RequestPty(system.GetTerminalType(), 80, 80, ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.IUTF8:         1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}); err != nil {
			return fmt.Errorf("failed to request pty: %w", err)
		}
	} else {
		logrus.Warnf("current terminal is not a tty, skip setting raw mode")
	}

	c.mySession.Stdout = os.Stdout
	c.mySession.Stderr = os.Stderr
	c.mySession.Stdin = os.Stdin

	if err = c.mySession.Shell(); err != nil {
		return fmt.Errorf("failed to start ssh command: %w", err)
	}

	context.AfterFunc(ctx, func() {
		if c.mySession != nil {
			logrus.Warnf("send signal %q to %q, cause by %v", c.signal, c.name, context.Cause(ctx))
			if err := c.mySession.Signal(c.signal); err != nil {
				logrus.Errorf("send signal %q to %q failed: %v", c.signal, c.name, err)
			}
		}
	})

	if err = c.mySession.Wait(); err != nil {
		return fmt.Errorf("ssh session exit with: %w", err)
	}

	return nil
}

func NewClient(addr, user string, port uint64, keyFile string, cmdline ...string) (*client, error) {
	f, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read ssh key failed: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key")
	}

	myClient := &client{
		config: &Config{
			Addr: addr,
			User: user,
			Port: port,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
		},
		signal: ssh.SIGKILL,
	}
	myClient.setCmdLine(cmdline[0], cmdline[1:])
	return myClient, nil
}
