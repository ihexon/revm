//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/cmd/revmcmd"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/revm"
	"linuxvm/pkg/system"
	"os"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "_cleanup" {
		if err := revm.RunCleanup(); err != nil {
			fmt.Fprintln(os.Stderr, "revm cleanup:", err)
			os.Exit(1)
		}
		return
	}
	terminal := system.SaveTerminal(int(os.Stdin.Fd()))
	err := revmcmd.Execute(context.Background(), os.Args)
	system.ProcessTerminal.Close()
	if system.RestoreTerminal(int(os.Stdin.Fd()), terminal) == nil {
		revm.TerminalRestored()
	}
	// Cancellation must not wait for a diagnostic write to a full stderr pipe.
	var canceled *execution.Canceled
	if err != nil && !errors.As(err, &canceled) {
		if message := commanderr.Diagnostic(err); message != "" {
			fmt.Fprintln(os.Stderr, "revm:", message)
		}
	}
	code := commanderr.ExitCode(err)
	libkrun.ExitProcess(code)
}
