//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var attachCommand = &cli.Command{
	Name:         define.FlagAttachMode,
	Usage:        "attach to a running VM session over SSH",
	UsageText:    "chroot attach [--pty] <session-id> [-- <command> [args...]]",
	Description:  "connect to a running VM session by id; launches an interactive shell with --pty or runs the specified command non-interactively",
	StopOnNthArg: &stopAfterSessionArg,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: define.FlagPTY, Usage: "allocate a pseudo-terminal and launch an interactive shell"},
		&cli.StringFlag{Name: define.FlagLogLevel, Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)", Value: "info"},
	},
	Action: runAttach,
}

var stopAfterSessionArg = 1

func runAttach(ctx context.Context, command *cli.Command) error {
	if err := setLogLevel(command.String(define.FlagLogLevel)); err != nil {
		return err
	}

	sessionID := command.Args().First()
	if sessionID == "" {
		return fmt.Errorf("no session id specified")
	}

	attached, err := revm.Attach(ctx, sessionID)
	if err != nil {
		return err
	}

	if command.Bool(define.FlagPTY) {
		return attached.Shell(ctx)
	}
	return attached.Run(ctx, command.Args().Tail()...)
}

func setLogLevel(level string) error {
	if level == "" {
		level = "info"
	}
	parsed, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}
	logrus.SetLevel(parsed)
	return nil
}
