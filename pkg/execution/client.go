package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}
type Options struct {
	IO
	OnStarted func(*Streams, Message) error
}

func Execute(ctx context.Context, conn net.Conn, req Request, opts Options) (result Result, retErr error) {
	defer func() {
		if result.Code != nil || result.Signal != "" {
			retErr = errors.Join(result.Err(), retErr)
		}
	}()
	s, started, err := Open(ctx, conn, req)
	if err != nil {
		return Result{}, err
	}
	defer s.Session.Close()
	if opts.OnStarted != nil {
		if err := opts.OnStarted(s, started); err != nil {
			return Result{}, err
		}
	}
	inputCtx, stopInput := context.WithCancel(context.Background())
	defer stopInput()
	inputDone := make(chan struct{})
	inputErrors := make(chan error, 1)
	go func() {
		defer close(inputDone)
		if opts.Stdin != nil {
			_, err := CopyContext(inputCtx, s.Stdin, opts.Stdin)
			var readErr *inputReadError
			if errors.As(err, &readErr) && !errors.Is(err, context.Canceled) {
				inputErrors <- fmt.Errorf("read command input: %w", err)
			}
		}
		_ = s.Stdin.CloseWrite()
	}()
	var copies sync.WaitGroup
	copies.Add(2)
	copyErrors := make(chan error, 2)
	for _, pair := range []struct {
		dst io.Writer
		src io.Reader
	}{{opts.Stdout, s.Stdout}, {opts.Stderr, s.Stderr}} {
		go func() {
			defer copies.Done()
			dst := pair.dst
			if dst == nil {
				dst = io.Discard
			}
			_, err := io.Copy(dst, pair.src)
			if err != nil {
				copyErrors <- err
			}
		}()
	}
	outputDone := make(chan struct{})
	go func() { copies.Wait(); close(outputDone) }()
	// Protect caller-owned writers from writes after return. The CLI owns this
	// goroutine and can exit immediately even when a downstream writer is stuck.
	defer func() { s.Session.Close(); <-outputDone }()
	type controlEvent struct {
		message Message
		err     error
	}
	messages := make(chan controlEvent, 4)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			var m Message
			err := s.Decoder.Decode(&m)
			// Preserve wire order, including errors after an exit result. A peer may
			// close immediately after its final message; only the consumer may stop
			// delivery of a message that has already been decoded.
			select {
			case messages <- controlEvent{message: m, err: err}:
			case <-readerDone:
				return
			}
			if err != nil || m.Type == "completed" {
				return
			}
		}
	}()

	outputFailure := func(err error) error {
		result.Output = "truncated"
		failure := fmt.Errorf("write command output: %w", err)
		if errors.Is(err, syscall.EPIPE) {
			return errors.Join(&Canceled{Signal: "SIGPIPE"}, failure)
		}
		return failure
	}
	completed, drained := false, false
	outputCh := outputDone
	for {
		if completed && drained {
			// Copies publish their errors before closing outputDone.
			for len(copyErrors) > 0 {
				return result, outputFailure(<-copyErrors)
			}
			stopInput()
			_ = s.Stdin.Close()
			select {
			case <-inputDone:
			case <-time.After(200 * time.Millisecond):
			}
			select {
			case err := <-inputErrors:
				return result, err
			default:
			}
			_ = s.Send(Message{Type: "ack", InstanceID: started.InstanceID, ExecID: started.ExecID})
			return result, nil
		}
		select {
		case <-outputCh:
			drained = true
			outputCh = nil
		case err := <-inputErrors:
			return result, err
		case <-ctx.Done():
			return result, context.Cause(ctx)
		case err := <-copyErrors:
			return result, outputFailure(err)
		case event := <-messages:
			if event.err != nil {
				return result, fmt.Errorf("execution disconnected before completion: %w", event.err)
			}
			m := event.message
			if m.Type == "error" {
				return result, errors.New(m.Error)
			}
			if m.Result != nil {
				result = *m.Result
			}
			if m.Type == "completed" {
				completed = true
				stopInput()
			}
		}
	}
}
