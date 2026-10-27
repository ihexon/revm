package revm

import (
	"context"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/service/guestcontrol"
	"linuxvm/pkg/system"
	"os"

	"golang.org/x/term"
)

func executeGuest(ctx context.Context, target guestcontrol.Target, cfg Config) (execution.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	useTTY := cfg.TTY == "always" || (cfg.TTY == "auto" && system.ForegroundTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())))
	req := cfg.Exec
	req.TTY, req.Term, req.Rows, req.Cols = useTTY, system.GetTerminalType(), 24, 80
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		req.Rows, req.Cols = uint16(h), uint16(w)
	}
	terminal := &system.ProcessTerminal
	defer terminal.Restore()
	controller := execution.ControllerFrom(ctx)
	if controller != nil {
		controller.SetTerminal(terminal.Event)
		defer controller.SetTerminal(nil)
	}
	opts := execution.Options{IO: execution.IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}}
	opts.OnStarted = func(s *execution.Streams, m execution.Message) error {
		return terminal.Start(useTTY, func() {
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err == nil {
				_ = s.Send(execution.Message{Type: "resize", InstanceID: m.InstanceID, ExecID: m.ExecID, Rows: uint16(h), Cols: uint16(w)})
			}
		})
	}

	return guestcontrol.StreamExec(ctx, target, req, opts)
}
