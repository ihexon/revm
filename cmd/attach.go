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

func attachConsole(ctx context.Context, command *cli.Command) error {
	rootfs := command.Args().First()
	if rootfs == "" {
		return fmt.Errorf("no rootfs specified, please provide the rootfs path, e.g. %s /path/to/rootfs", command.Name)
	}

	vmc, err := define.LoadVMCFromFile(filepath.Join(rootfs, define.VMConfigFile))
	if err != nil {
		return err
	}

	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	if command.Args().Len() < 2 {
		return fmt.Errorf("no cmdline specified, please provide the cmdline, e.g. %s /path/to/rootfs /bin/bash", command.Name)
	}

	cmdline := command.Args().Tail()

	// First we maka a ssh client configure, remember it just a configure store the information the ssh client actually needed
	cfg := ssh.NewCfg(define.DefaultGuestAddr, vmc.SSHInfo.User, vmc.SSHInfo.Port, vmc.SSHInfo.HostSSHKeyPairFile)
	defer cfg.CleanUp.CleanIfErr(&err)

	// Set the cmdline
	cfg.SetCmdLine(cmdline[0], cmdline[1:])

	if command.Bool(define.FlagPTY) {
		cfg.SetPty(true)
	}

	// Connect to the ssh server over gvproxy vsock, we use the gvproxy endpoint to connect to the ssh server
	if err = cfg.Connect(ctx, endpoint.Path); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", endpoint.Path, err)
	}

	// make stdout/stderr pipe, so we can get the output of the cmdline in realtime
	if err = cfg.WriteOutputTo(os.Stdout, os.Stderr); err != nil {
		return fmt.Errorf("failed to make std pipe: %w", err)
	}

	// if enable pty, we need to request a pty
	if cfg.IsPty() {
		resetFunc, err := cfg.RequestPTY(ctx)
		if err != nil {
			return fmt.Errorf("failed to request pty: %w", err)
		}
		defer resetFunc()
	}

	return cfg.Run(ctx)
}
