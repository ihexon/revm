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
	StopTimeout time.Duration
	Controller  *Controller
	OnStarted   func(*Streams, Message) error
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
	// Normal completion waits for outputDone below. Cancellation may abandon a
	// blocked caller-owned writer; the one-shot CLI process owns its lifetime.
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

	var cancelErr error
	var force <-chan struct{}
	if opts.Controller != nil {
		force = opts.Controller.Force()
	}
	var timer *time.Timer
	var deadline <-chan time.Time
	cancelCh := ctx.Done()
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	cancelExec := func(err error) {
		if cancelErr != nil {
			return
		}
		cancelErr = err
		stopInput()
		sig := "SIGTERM"
		grace := opts.StopTimeout
		var canceled *Canceled
		if errors.As(err, &canceled) {
			sig = canceled.Signal
			grace = max(0, time.Until(canceled.Deadline))
		}
		_ = s.Send(Message{Type: "cancel", InstanceID: started.InstanceID, ExecID: started.ExecID, Signal: sig, GraceMillis: grace.Milliseconds()})
		timer = time.NewTimer(grace + 3*time.Second)
		deadline = timer.C
	}
	outputFailure := func(err error) {
		failure := fmt.Errorf("write command output: %w", err)
		if errors.Is(err, syscall.EPIPE) {
			if opts.Controller != nil {
				opts.Controller.Request("SIGPIPE")
				if cause := opts.Controller.Cause(); cause != nil {
					failure = errors.Join(cause, failure)
				}
			} else {
				now := time.Now()
				failure = errors.Join(&Canceled{Signal: "SIGPIPE", At: now, Deadline: now.Add(opts.StopTimeout)}, failure)
			}
		}
		cancelExec(failure)
	}
	completed, drained := false, false
	outputCh := outputDone
	for {
		if completed && drained {
			// Copies publish their errors before closing outputDone.
			for len(copyErrors) > 0 {
				outputFailure(<-copyErrors)
			}
			stopInput()
			_ = s.Stdin.Close()
			select {
			case <-inputDone:
			case <-time.After(200 * time.Millisecond):
			}
			select {
			case err := <-inputErrors:
				cancelErr = errors.Join(cancelErr, err)
			default:
			}
			_ = s.Send(Message{Type: "ack", InstanceID: started.InstanceID, ExecID: started.ExecID})
			return result, cancelErr
		}
		select {
		case <-outputCh:
			drained = true
			outputCh = nil
		case err := <-inputErrors:
			cancelExec(err)
		case <-force:
			force = nil
			_ = s.Send(Message{Type: "cancel", InstanceID: started.InstanceID, ExecID: started.ExecID, Signal: "SIGKILL", GraceMillis: 0})
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(3 * time.Second)
			deadline = timer.C
		case <-cancelCh:
			cancelCh = nil
			cancelExec(context.Cause(ctx))
		case err := <-copyErrors:
			outputFailure(err)
		case <-deadline:
			return result, errors.Join(cancelErr, errors.New("execution cleanup deadline exceeded"))
		case event := <-messages:
			if event.err != nil {
				return result, errors.Join(cancelErr, fmt.Errorf("execution disconnected before completion: %w", event.err))
			}
			m := event.message
			if m.Type == "error" {
				return result, errors.New(m.Error)
			}
			if m.Result != nil {
				result = *m.Result
				if m.Type == "exit" && opts.Controller != nil {
					opts.Controller.Complete()
				}
			}
			if m.Type == "completed" {
				completed = true
				stopInput()
			}
		}
	}
}
