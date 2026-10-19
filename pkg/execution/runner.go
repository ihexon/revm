//go:build darwin || linux

package execution

import (
	"context"
	"github.com/creack/pty"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// Serve owns one execution, including cleanup when its transport disconnects.
func Serve(ctx context.Context, conn net.Conn, instance string, registry *Registry) {
	handshake, cancel := context.WithTimeout(ctx, 10*time.Second)
	s, err := streams(handshake, conn, true)
	cancel()
	if err != nil {
		return
	}
	defer s.Session.Close()
	_ = s.Control.SetReadDeadline(time.Now().Add(10 * time.Second))
	var req Request
	if s.Decoder.Decode(&req) != nil {
		return
	}
	_ = s.Control.SetReadDeadline(time.Time{})
	if req.Version != 2 || req.InstanceID != instance || len(req.Argv) == 0 || !registry.Claim(req.RequestID) {
		_ = s.Send(Message{Type: "error", Error: "invalid, duplicate or stale execution request"})
		return
	}
	cmd, input, outR, errR, terminal, startErr := startProcess(req)
	defer outR.Close()
	defer errR.Close()
	if terminal != nil {
		defer terminal.Close()
	}
	execID := req.RequestID
	if s.Send(Message{Type: "started", InstanceID: instance, ExecID: execID}) != nil {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
		}
		input.Close()
		return
	}
	if startErr != nil {
		input.Close()
		s.Stdout.CloseWrite()
		s.Stderr.CloseWrite()
		r := FromProcess(startErr)
		_ = s.Send(Message{Type: "exit", Result: &r})
		_ = s.Send(Message{Type: "completed", Result: &r})
		waitAck(s)
		return
	}
	go func() { _, _ = io.Copy(input, s.Stdin); input.Close() }()
	var copies sync.WaitGroup
	copies.Add(2)
	go func() { defer copies.Done(); _, _ = io.Copy(s.Stdout, outR); _ = s.Stdout.CloseWrite() }()
	go func() { defer copies.Done(); _, _ = io.Copy(s.Stderr, errR); _ = s.Stderr.CloseWrite() }()
	outputDone := make(chan struct{})
	go func() { copies.Wait(); close(outputDone) }()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	controls := make(chan Message, 8)
	go func() {
		defer close(controls)
		for {
			var m Message
			if s.Decoder.Decode(&m) != nil {
				return
			}
			select {
			case controls <- m:
			case <-s.Session.CloseChan():
				return
			}
		}
	}()
	var timer *time.Timer
	var deadline <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	cancelExec := func(sig string, grace time.Duration) {
		if timer != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return
		}
		signal, ok := Signal(sig)
		if !ok {
			signal = syscall.SIGTERM
		}
		_ = syscall.Kill(-cmd.Process.Pid, signal)
		timer = time.NewTimer(max(0, grace))
		deadline = timer.C
	}
	var waitErr error
running:
	for {
		select {
		case waitErr = <-waited:
			break running
		case m, ok := <-controls:
			if !ok {
				controls = nil
				cancelExec("SIGTERM", 10*time.Second)
				continue
			}
			if m.InstanceID != instance || m.ExecID != execID {
				continue
			}
			if m.Type == "cancel" {
				cancelExec(m.Signal, time.Duration(m.GraceMillis)*time.Millisecond)
			}
			if m.Type == "resize" && terminal != nil && m.Rows > 0 && m.Cols > 0 {
				_ = pty.Setsize(terminal, &pty.Winsize{Rows: m.Rows, Cols: m.Cols})
			}
			if m.Type == "signal" {
				if sig, ok := Signal(m.Signal); ok {
					_ = syscall.Kill(-cmd.Process.Pid, sig)
				}
			}
		case <-ctx.Done():
			cancelExec("SIGTERM", 0)
		case <-deadline:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			deadline = nil
		}
	}
	input.Close()
	// The initial implementation owns the process group. The cgroup boundary
	// added with signal/terminal support covers descendants which call setsid.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	result := FromProcess(waitErr)
	_ = s.Send(Message{Type: "exit", Result: &result})
	select {
	case <-outputDone:
	case <-time.After(3 * time.Second):
		result.Output = "truncated"
		result.Kind = "output"
		result.Message = "output drain timed out"
		outR.Close()
		errR.Close()
		s.Stdout.Close()
		s.Stderr.Close()
	}
	_ = s.Send(Message{Type: "completed", Result: &result})
	// The receiver acknowledges only after both streams have reached EOF.
	select {
	case <-controls:
	case <-s.Session.CloseChan():
	case <-time.After(3 * time.Second):
	}
}

func waitAck(s *Streams) {
	_ = s.Control.SetReadDeadline(time.Now().Add(3 * time.Second))
	var m Message
	_ = s.Decoder.Decode(&m)
}
