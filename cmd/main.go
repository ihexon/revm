//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run a linux shell in 1 second",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "run a linux shell in 1 second",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  define.FlagReportURL,
				Usage: "report virtual machine events to this endpoints, eg: unix:///var/run/events.sock or tcp://192.168.1.252:8888",
			},
			&cli.StringFlag{
				Name:  define.FlagLogLevel,
				Usage: "set log level (trace, debug, info, warn, error, fatal, panic)",
				Value: "warn",
			},
			&cli.StringFlag{
				Name:  define.FlagSaveLogTo,
				Usage: "save log to file",
			},
			&cli.StringFlag{
				Name:  define.FlagWorkspace,
				Usage: "workspace path",
				Value: "/tmp/revm-workspace",
			},
		},
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	// Orphan process detection mechanism
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() == 1 {
					cancel()
					return
				}
			}
		}
	}()

	if err := app.Run(ctx, os.Args); err != nil {
		logrus.Fatal(err)
	}
}
