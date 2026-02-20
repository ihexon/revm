//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service"
	"linuxvm/pkg/vmbuilder"
	"net/http"
	"path/filepath"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var AttachConsole = cli.Command{
	Name:        "attach",
	Usage:       "attach to a running VM and execute a command",
	UsageText:   "attach [OPTIONS] <workspace> [-- cmdline]",
	Description: "attach to the console of the running VM, provide the interactive shell",
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
	workspace := command.Args().First()
	enablePTY := command.Bool(define.FlagPTY)
	cmdline := command.Args().Tail()

	if workspace == "" {
		return fmt.Errorf("no workspace specified, please provide the workspace path")
	}

	// Extract command line arguments
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}
	logrus.Infof("run cmdline: %v", cmdline)

	// Fetch Machine from ignition server
	ignAddr := vmbuilder.NewPathManager(workspace).GetIgnAddr()

	client := network.NewUnixClient(ignAddr)
	defer client.Close()

	resp, err := client.Get(define.RestAPIVMConfigURL).Do(ctx) //nolint:bodyclose
	if err != nil {
		return fmt.Errorf("failed to fetch vmconfig from ignition server: %w", err)
	}
	defer network.CloseResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ignition server returned status %d", resp.StatusCode)
	}

	var vmc define.Machine
	if err := json.NewDecoder(resp.Body).Decode(&vmc); err != nil {
		return fmt.Errorf("failed to decode vmconfig: %w", err)
	}

	sshClient, err := service.MakeSSHClient(ctx, &vmc)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	if enablePTY {
		return sshClient.Shell(ctx)
	}

	return sshClient.Run(ctx, shellescape.QuoteCommand(cmdline))
}
