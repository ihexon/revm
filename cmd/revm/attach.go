//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
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
	Name:        define.FlagAttachMode,
	Usage:       "attach to a running VM and execute a command over SSH",
	UsageText:   "attach [--pty] <session-name> [-- <command> [args...]]",
	Description: "connect to a running VM session by name via SSH; the session-name maps to /tmp/.revm-<name>; launches an interactive shell (--pty) or runs the specified command non-interactively; defaults to /bin/sh if no command is given",
	Action:      attachConsole,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  define.FlagPTY,
			Usage: "allocate a pseudo-terminal and launch an interactive shell, without this flag the command runs non-interactively with plain stdin/stdout/stderr",
			Value: false,
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagReportURL,
			Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888); events include: ConfigureVirtualMachine, StartVirtualNetwork, StartIgnitionServer, StartVirtualMachine, GuestNetworkReady, GuestSSHReady, GuestPodmanReady, Exit, Error",
		},
	},
}

func attachConsole(ctx context.Context, command *cli.Command) error {
	if err := SetupBasicLogger(command.String(define.FlagLogLevel)); err != nil {
		return fmt.Errorf("failed to setup basic logger: %w", err)
	}

	event.Setup(command.String(define.FlagReportURL), event.Attach)

	if err := LaunchCleaner(""); err != nil {
		logrus.Warnf("failed to start clean helper: %v", err)
	}

	name := command.Args().First()
	enablePTY := command.Bool(define.FlagPTY)
	cmdline := command.Args().Tail()

	if name == "" {
		return fmt.Errorf("no session name specified, please provide the session name")
	}

	workspace := fmt.Sprintf("/tmp/.revm-%s", name)

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
