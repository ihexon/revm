//go:build !windows

package system

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

var isTerminal bool = isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

type stdinState struct {
	PreviousState *term.State
}

func makeStdinRaw() (*stdinState, error) {
	previousState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("terminal make raw failed: %v", err)
	}
	return &stdinState{previousState}, nil
}

func resetStdin(s *stdinState) {
	if s.PreviousState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), s.PreviousState)
		s.PreviousState = nil
	}
}

func getTerminalSize() (int, int, error) {
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func OnTerminalResize(setTerminalSize func(), ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func(ctx context.Context) {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				setTerminalSize()
			}
		}
	}(ctx)

	ch <- syscall.SIGWINCH
}

// TerminalState wraps the terminal state for restoration
type TerminalState struct {
	state *stdinState
}

// MakeStdinRaw puts stdin into raw mode and returns state for restoration
func MakeStdinRaw() (*TerminalState, error) {
	state, err := makeStdinRaw()
	if err != nil {
		return nil, err
	}
	return &TerminalState{state: state}, nil
}

// ResetStdin restores stdin to its original state
func ResetStdin(s *TerminalState) {
	if s != nil && s.state != nil {
		resetStdin(s.state)
	}
}

// GetTerminalSize returns the current terminal dimensions
func GetTerminalSize() (width, height int, err error) {
	return getTerminalSize()
}

// IsTerminal returns true if stdin is a terminal
func IsTerminal() bool {
	return isTerminal
}

// GetTerminalType returns the TERM environment variable or a default
func GetTerminalType() string {
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
	}
	return termEnv
}
