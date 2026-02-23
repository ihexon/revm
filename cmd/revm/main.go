//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run Linux microVMs on macOS/arm64 using libkrun",
		UsageText:                 os.Args[0] + " [global flags] <command> [flags]",
		Description:               "revm boots lightweight Linux microVMs backed by Apple Hypervisor via libkrun; supports rootfs mode (chroot-like) and container mode (podman-compatible API)",
		DisableSliceFlagSeparator: true,
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
		},
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}
