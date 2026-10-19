//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/cmd/revmcmd"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/libkrun"
	"os"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "_vmm" {
		if err := libkrun.RunChild(); err != nil {
			fmt.Fprintln(os.Stderr, "revm VMM:", err)
			os.Exit(1)
		}
		return
	}
	if err := revmcmd.NewApp("revm").Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "revm:", err)
		os.Exit(commanderr.ExitCode(err))
	}
}
