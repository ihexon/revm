//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func setupLoggerEarly() {
	f, _ := os.OpenFile("/tmp/vm-early-64b1c33df749.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	w := io.MultiWriter(os.Stderr, f)
	logrus.SetOutput(w)
}

func main() {
	setupLoggerEarly()

	logrus.Infof("[early-log] fully qualified command: %s", os.Args)
	logrus.Infof("[early-log] version: %v", showVersionAndOSInfo())

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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		logrus.Error(err)
		os.Exit(1)
	}
}
