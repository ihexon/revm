//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/ssh"
	"net/url"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	gossh "golang.org/x/crypto/ssh"
)

var AttachConsole = cli.Command{
	Name:        "attach",
	Usage:       "attach to the guest and running command",
	UsageText:   "attach [OPTIONS] [rootfs] [cmdline]",
	Description: "attach to the console of the running rootfs, provide the interactive shell of the rootfs",
	Action:      attachConsole,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    define.FlagPTY,
			Aliases: []string{"tty"},
			Usage:   "enable pseudo-terminal",
			Value:   false,
		},
	},
}

func attachConsole(ctx context.Context, command *cli.Command) (err error) {
	// Parse and validate arguments
	rootfs := command.Args().First()
	if rootfs == "" {
		return fmt.Errorf("no rootfs specified, please provide the rootfs path, e.g. %s /path/to/rootfs", command.Name)
	}

	if command.Args().Len() < 2 {
		return fmt.Errorf("no cmdline specified, please provide the cmdline, e.g. %s /path/to/rootfs /bin/bash", command.Name)
	}

	// Load VM configuration
	vmc, err := define.LoadVMCFromFile(filepath.Join(rootfs, define.VMConfigFilePathInGuest))
	if err != nil {
		return err
	}

	// Extract command line arguments
	cmdline := command.Args().Tail()

	// Parse gvproxy endpoint
	endpoint, err := url.Parse(vmc.GvisorTapVsockEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	// Configure SSH client
	clientCfg := ssh.NewClientConfig(
		define.GuestIP,
		uint16(define.GuestSSHServerPort),
		define.DefaultGuestUser,
		vmc.SSHInfo.HostSSHPrivateKeyFile,
	).WithGVProxySocket(endpoint.Path)

	// Create SSH client
	client, err := ssh.NewClient(ctx, clientCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint.Path, err)
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	// Check if PTY mode is enabled
	enablePTY := command.Bool(define.FlagPTY)

	if enablePTY {
		// PTY mode: directly assign I/O streams, then request PTY
		if err := session.SetStdin(os.Stdin); err != nil {
			return err
		}
		if err := session.SetStdout(os.Stdout); err != nil {
			return fmt.Errorf("failed to setup stdout: %w", err)
		}
		if err := session.SetStderr(os.Stderr); err != nil {
			return fmt.Errorf("failed to setup stderr: %w", err)
		}

		if err := session.RequestPTY(ctx, "", 0, 0); err != nil {
			return fmt.Errorf("failed to request pty: %w", err)
		}
	} else {
		// Non-PTY mode: use pipes with async I/O copying
		if err := session.SetupPipes(nil, os.Stdout, os.Stderr); err != nil {
			return fmt.Errorf("failed to setup pipes: %w", err)
		}
	}

	// Set up context cancellation to send SIGTERM
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-runCtx.Done()
		if ctx.Err() != nil {
			_ = session.Signal(gossh.SIGTERM)
		}
	}()

	// Execute command
	return session.Run(ctx, cmdline...)
}
