//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		DisableSliceFlagSeparator: true,
	}

	app.Commands = []*cli.Command{
		&initCmd,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}
