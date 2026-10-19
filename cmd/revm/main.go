//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/cmd/revmcmd"
	"linuxvm/pkg/commanderr"
	"os"
)

func main() {
	if err := revmcmd.NewApp("revm").Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "revm:", err)
		os.Exit(commanderr.ExitCode(err))
	}
}
