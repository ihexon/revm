//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run a linux shell in 1 second",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "run a linux shell in 1 second",
		Before:                    earlyStage,
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:   define.FlagVMMProvider,
				Usage:  "vmm provider, support libkrun and vfkit for now, default is libkrun",
				Hidden: true,
			},
			&cli.StringFlag{
				Name: define.FlagRestAPIListenAddr,
				Usage: "listen for REST API requests on the given address, support http or unix socket address," +
					" e.g. http://127.0.0.1:8080 or unix:///tmp/restapi.sock",
			},
			&cli.StringFlag{
				Name:  define.FlagLogLevel,
				Usage: "set log level (trace, debug, info, warn, error, fatal, panic)",
				Value: "warn",
			},
		},
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)
	if err := app.Run(ctx, os.Args); err != nil {
		logrus.Fatal(err)
	}
}
