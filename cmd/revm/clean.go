package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var cleanResource = cli.Command{
	Name:        define.FlagClean,
	Usage:       "delete workspace",
	Description: "run as a standalone process, with for main process exit and start clean all resource",
	Action:      cleanAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  define.FlagReportURL,
			Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888); events include: ConfigureVirtualMachine, StartVirtualNetwork, StartIgnitionServer, StartVirtualMachine, GuestNetworkReady, GuestSSHReady, GuestPodmanReady, Exit, Error",
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "directory for VM runtime state: Unix sockets (podman API, gvproxy ctl, ignition), SSH keys, guest logs, and auto-created disk images; cannot be the home directory",
		},
	},
}

func cleanAction(ctx context.Context, command *cli.Command) error {
	if err := SetupBasicLogger(command.String(define.FlagLogLevel)); err != nil {
		return fmt.Errorf("failed to setup basic logger: %w", err)
	}

	event.Setup(command.String(define.FlagReportURL), event.Clean)

	workspace := command.String(define.FlagWorkspace)
	if workspace == "" {
		return nil
	}

	workspace, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return err
	}

	for {
		ppid := os.Getppid()
		if ppid == 1 {
			logrus.Infof("current PPID=%d, remove workspace %q", ppid, workspace)
			if _, err = os.Stat(workspace); err == nil {
				return os.RemoveAll(workspace)
			}
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}
