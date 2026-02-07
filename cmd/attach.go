//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/service"
	"linuxvm/pkg/vmconfig"
	"path/filepath"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
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
	rootfsPath := command.Args().First()
	enablePTY := command.Bool(define.FlagPTY)
	cmdline := command.Args().Tail()

	if rootfsPath == "" {
		return fmt.Errorf("no rootfs specified, please provide the rootfs path")
	}

	rootfsPath, err := filepath.Abs(filepath.Clean(rootfsPath))
	if err != nil {
		return err
	}

	// Extract command line arguments
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}
	logrus.Infof("run cmdline: %v", cmdline)

	// Load VM configuration
	vmc, err := vmconfig.LoadVMCFromFile(filepath.Join(rootfsPath, define.VMConfigFilePathInGuest))
	if err != nil {
		return err
	}

	sshClient, err := service.MakeSSHClient(ctx, (*define.VMConfig)(vmc))
	if err != nil {
		return err
	}
	defer sshClient.Close()

	if enablePTY {
		return sshClient.Shell(ctx)
	}

	return sshClient.Run(ctx, shellescape.QuoteCommand(cmdline))
}
