//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/cmd/internal/revmcmd"
	"os"

	"github.com/sirupsen/logrus"
)

func main() {
	if err := revmcmd.NewApp("revm").Run(context.Background(), os.Args); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}
