package revm

import (
	"context"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/service/guestcontrol"
	"linuxvm/pkg/system"
	"os"
	"time"

	"golang.org/x/term"
)

func executeGuest(ctx context.Context, target guestcontrol.Target, cfg Config) (execution.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	useTTY := cfg.PTY || (term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())))
	req := execution.Request{Argv: cfg.Command, Env: cfg.Env, WorkDir: cfg.WorkDir, TTY: useTTY, Term: system.GetTerminalType(), Rows: 24, Cols: 80}
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		req.Rows, req.Cols = uint16(h), uint16(w)
	}
	var state *term.State
	defer func() {
		if state != nil {
			_ = term.Restore(int(os.Stdin.Fd()), state)
		}
	}()
	opts := execution.Options{IO: execution.IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, StopTimeout: 10 * time.Second}
	opts.OnStarted = func(s *execution.Streams, m execution.Message) error {
		if !useTTY {
			return nil
		}
		if term.IsTerminal(int(os.Stdin.Fd())) {
			var err error
			state, err = term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
		}
		system.OnTerminalResize(func() {
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err == nil {
				_ = s.Send(execution.Message{Type: "resize", InstanceID: m.InstanceID, ExecID: m.ExecID, Rows: uint16(h), Cols: uint16(w)})
			}
		}, ctx)
		return nil
	}
	return guestcontrol.StreamExec(ctx, target, req, opts)
}
