//go:build darwin || linux

package execution

import (
	"context"
	"github.com/creack/pty"
	"io"
	"net"
	"sync"
	"time"
)

// Serve owns one execution, including cleanup when its transport disconnects.
func Serve(ctx context.Context, conn net.Conn, instance string, defaultEnv ...string) {
	handshake, cancel := context.WithTimeout(ctx, 10*time.Second)
	s, err := streams(handshake, conn, true)
	cancel()
	if err != nil {
		return
	}
	defer s.Session.Close()
	stopHandshake := context.AfterFunc(ctx, func() { s.Session.Close() })
	defer stopHandshake()
	_ = s.Control.SetReadDeadline(time.Now().Add(10 * time.Second))
	var req Request
	if s.Decoder.Decode(&req) != nil {
		return
	}
	stopHandshake()
	if ctx.Err() != nil {
		return
	}
	_ = s.Control.SetReadDeadline(time.Time{})
	if req.Version != ProtocolVersion || req.InstanceID != instance || len(req.Argv) == 0 {
		_ = s.Send(Message{Type: "error", Error: "invalid or stale execution request"})
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
	if s.Send(Message{Type: "started"}) != nil {
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
		_ = s.Send(Message{Type: "completed", Result: &r})
		waitAck(ctx, s, controls)
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
				_ = owned.Kill()
				continue
			}
			if m.Type == "resize" && terminal != nil && m.Rows > 0 && m.Cols > 0 {
				_ = pty.Setsize(terminal, &pty.Winsize{Rows: m.Rows, Cols: m.Cols})
			}
		case <-cancelCh:
			cancelCh = nil
			_ = owned.Kill()
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
	// Normal output follows downstream backpressure, regardless of duration.
	// Only a cancellation or disconnect abandons the remaining output.
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
		case _, ok := <-controls:
			if !ok {
				return
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
	waitAck(ctx, s, controls)
}

func waitAck(ctx context.Context, s *Streams, controls <-chan Message) {
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
			if m.Type == "ack" {
				return
			}
		}
	}
}
