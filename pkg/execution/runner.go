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
func Serve(ctx context.Context, conn net.Conn, instance string, registry *Registry, defaultEnv ...string) {
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
	req.Env = append(append([]string(nil), defaultEnv...), req.Env...)
	cmd, input, outR, errR, terminal, owned, startErr := startProcess(req)
	if owned != nil {
		defer owned.Close()
	}
	defer outR.Close()
	defer errR.Close()
	if terminal != nil {
		defer terminal.Close()
	}
	execID := req.RequestID
	if s.Send(Message{Type: "started", InstanceID: instance, ExecID: execID}) != nil {
		if cmd.Process != nil {
			_ = owned.Kill()
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
		waitAck(ctx, s, controls, instance, execID)
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
	var timer *time.Timer
	var deadline <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	cancelExec := func(sig string, grace time.Duration) {
		if timer != nil {
			_ = owned.Kill()
			return
		}
		signal, ok := Signal(sig)
		if !ok {
			signal = syscall.SIGTERM
		}
		owned.Signal(signal)
		timer = time.NewTimer(max(0, grace))
		deadline = timer.C
	}
	cancelCh := ctx.Done()
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
					owned.Signal(sig)
				}
			}
		case <-cancelCh:
			cancelCh = nil
			cancelExec("SIGTERM", 0)
		case <-deadline:
			_ = owned.Kill()
			deadline = nil
		}
	}
	input.Close()
	// Kill the entire execution boundary, including descendants using setsid.
	cleanupErr := owned.Kill()
	if closeErr := owned.Close(); cleanupErr == nil {
		cleanupErr = closeErr
	}
	result := FromProcess(waitErr)
	if cleanupErr != nil {
		result.Cleanup = "failed"
		result.Message = cleanupErr.Error()
	}
	exitResult := result
	exitResult.Output = "pending"
	_ = s.Send(Message{Type: "exit", Result: &exitResult})
	// Normal output follows downstream backpressure, regardless of duration.
	// Only a cancellation or disconnect abandons the remaining output.
	if timer != nil {
		timer.Stop()
		timer = time.NewTimer(3 * time.Second)
		deadline = timer.C
	}
draining:
	for {
		select {
		case <-outputDone:
			break draining
		case <-ctx.Done():
			result.Output = "truncated"
			break draining
		case <-s.Session.CloseChan():
			return
		case <-deadline:
			result.Output = "truncated"
			break draining
		case m, ok := <-controls:
			if !ok {
				return
			}
			if m.InstanceID == instance && m.ExecID == execID && m.Type == "cancel" {
				result.Output = "truncated"
				break draining
			}
		}
	}
	if result.Output == "truncated" {
		outR.Close()
		errR.Close()
		s.Stdout.Close()
		s.Stderr.Close()
	}
	_ = s.Send(Message{Type: "completed", Result: &result})
	waitAck(ctx, s, controls, instance, execID)
}

func waitAck(ctx context.Context, s *Streams, controls <-chan Message, instance, execID string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Session.CloseChan():
			return
		case m, ok := <-controls:
			if !ok {
				return
			}
			if m.InstanceID == instance && m.ExecID == execID && (m.Type == "ack" || m.Type == "cancel") {
				return
			}
		}
	}
}
