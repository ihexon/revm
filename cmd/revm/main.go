//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func setupLoggerEarly() {
	f, _ := os.OpenFile("/tmp/vm-early.log", os.O_CREATE|os.O_WRONLY, 0644)
	w := io.MultiWriter(os.Stderr, f)
	logrus.SetOutput(w)
}

func main() {
	setupLoggerEarly()

	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run Linux microVMs on macOS/arm64 using libkrun",
		UsageText:                 os.Args[0] + " [global flags] <command> [flags]",
		Description:               "revm boots lightweight Linux microVMs backed by Apple Hypervisor via libkrun; supports rootfs mode (chroot-like) and container mode (podman-compatible API)",
		DisableSliceFlagSeparator: true,
	}

	app.Commands = []*cli.Command{
		&initCommand,
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}
