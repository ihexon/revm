package revmcmd

import (
	"context"
	"errors"
	"github.com/sirupsen/logrus"
	"io"
	"linuxvm/pkg/execution"
	"os"
	"os/signal"
	"syscall"
)

func Execute(ctx context.Context, args []string) error {
	logrus.SetOutput(io.Discard)
	ctx, controller := execution.WithController(ctx)
	ch := make(chan os.Signal, 8)
	signals := []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGPIPE, syscall.SIGWINCH, syscall.SIGTSTP, syscall.SIGCONT}
	if !signal.Ignored(syscall.SIGHUP) {
		signals = append(signals, syscall.SIGHUP)
	}
	signal.Notify(ch, signals...)
	done := make(chan struct{})
	defer close(done)
	defer signal.Stop(ch)
	go func() {
		for {
			select {
			case <-done:
				return
			case s := <-ch:
				name := execution.SignalName(s.(syscall.Signal))
				switch s {
				case syscall.SIGWINCH, syscall.SIGCONT:
					controller.Terminal(name)
				case syscall.SIGTSTP:
					controller.Terminal(name)
					_ = syscall.Kill(os.Getpid(), syscall.SIGSTOP)
				default:
					controller.Request(name)
				}
			}
		}
	}()
	err := NewApp("revm").Run(ctx, args)
	if cause := controller.Cause(); cause != nil {
		return errors.Join(cause, err)
	}
	return err
}
