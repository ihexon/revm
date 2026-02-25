package main

import (
	"context"
	"linuxvm/pkg/define"
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
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "workspace stores user data,ssh keys, logs, and temporary Unix sockets",
		},
	},
	Action: cleanAction,
}

func cleanAction(ctx context.Context, command *cli.Command) error {
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
