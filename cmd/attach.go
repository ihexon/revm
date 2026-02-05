//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/service"

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

func attachConsole(ctx context.Context, command *cli.Command) (err error) {
	return service.AttachGuestConsole(ctx, command.Args().First(), command.Bool(define.FlagPTY), command.Args().Tail()...)
}
