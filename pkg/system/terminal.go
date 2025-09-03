package system

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

type StdinState struct {
	State *term.State
}

func GetTerminalSize() (int, int, error) {
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func MakeStdinRaw() (*StdinState, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("terminal make raw failed: %v", err)
	}
	return &StdinState{state}, nil
}

func ResetStdin(s *StdinState) {
	if s.State != nil {
		_ = term.Restore(int(os.Stdin.Fd()), s.State)
		s.State = nil
	}
}

func OnTerminalResize(ctx context.Context, setTerminalSize func(int, int)) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	if width, height, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		setTerminalSize(width, height)
	}

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				logrus.Infof("terminal resize context done")
				return
			case <-ch:
				if width, height, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
					setTerminalSize(width, height)
				}
			}
		}
	}()
}

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func GetTerminalType() string {
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
	}

	return termEnv
}
