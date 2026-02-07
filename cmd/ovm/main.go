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
		Usage:                     "oomol virtual machine",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "Quickly launch a Linux container environment",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  define.FlagReportURL,
				Usage: "report virtual machine events to this endpoints, eg: unix:///var/run/events.sock",
			},
			&cli.StringFlag{
				Name:  define.FlagLogLevel,
				Usage: "set log level (trace, debug, info, warn, error, fatal, panic), logs also save into workspace",
				Value: "info",
			},
			&cli.StringFlag{
				Name:     define.FlagWorkspace,
				Usage:    "workspace stores user data,ssh keys, logs, and temporary Unix sockets",
				Required: true,
			},
		},
	}

	app.Commands = []*cli.Command{
		&initCmd,
		&startCmd,
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
					logrus.Warn("parent process exited, shutting down...")
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
