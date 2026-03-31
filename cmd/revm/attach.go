//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var AttachConsole = cli.Command{
	Name:        define.FlagAttachMode,
	Usage:       "attach to a running VM and execute a command over SSH",
	UsageText:   "attach [--pty] <session-name> [-- <command> [args...]]",
	Description: "connect to a running VM session by name via SSH; the session-name maps to /tmp/<name>; launches an interactive shell (--pty) or runs the specified command non-interactively; defaults to /bin/sh if no command is given",
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
	},
}

func attachConsole(ctx context.Context, command *cli.Command) error {
	level := command.String(define.FlagLogLevel)
	if level == "" {
		level = "info"
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("failed to parse log level: %w", err)
	}
	logrus.SetLevel(l)

	name := command.Args().First()
	enablePTY := command.Bool(define.FlagPTY)
	cmdline := command.Args().Tail()

	if name == "" {
		return fmt.Errorf("no session name specified, please provide the session name")
	}

	attached, err := librevm.Attach(ctx, name)
	if err != nil {
		return err
	}

	logrus.Infof("run cmdline: %v", cmdline)

	if enablePTY {
		return attached.Shell(ctx)
	}

	return attached.Run(ctx, cmdline...)
}
