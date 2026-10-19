package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type IO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}
type Options struct {
	IO
	StopTimeout time.Duration
	OnStarted   func(*Streams, Message) error
}

func Execute(ctx context.Context, conn net.Conn, req Request, opts Options) (Result, error) {
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
	go func() {
		defer close(inputDone)
		if opts.Stdin != nil {
			_, _ = CopyContext(inputCtx, s.Stdin, opts.Stdin)
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
	messages := make(chan Message, 4)
	readErrors := make(chan error, 1)
	go func() {
		for {
			var m Message
			if err := s.Decoder.Decode(&m); err != nil {
				readErrors <- err
				return
			}
			select {
			case messages <- m:
			case <-s.Session.CloseChan():
				return
			}
			if m.Type == "completed" {
				return
			}
		}
	}()
	var result Result
	var cancelErr error
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
		_ = s.Send(Message{Type: "cancel", InstanceID: started.InstanceID, ExecID: started.ExecID, Signal: "SIGTERM", GraceMillis: opts.StopTimeout.Milliseconds()})
		timer = time.NewTimer(opts.StopTimeout + 3*time.Second)
		deadline = timer.C
	}
	for {
		select {
		case <-cancelCh:
			cancelCh = nil
			cancelExec(context.Cause(ctx))
		case err := <-copyErrors:
			cancelExec(fmt.Errorf("write command output: %w", err))
		case <-deadline:
			return result, errors.Join(cancelErr, errors.New("execution cleanup deadline exceeded"))
		case err := <-readErrors:
			return result, errors.Join(cancelErr, fmt.Errorf("execution disconnected before completion: %w", err))
		case m := <-messages:
			if m.Type == "error" {
				return result, errors.New(m.Error)
			}
			if m.Result != nil {
				result = *m.Result
			}
			if m.Type == "completed" {
				stopInput()
				select {
				case <-outputDone:
				case <-deadline:
					return result, errors.Join(cancelErr, errors.New("output drain deadline exceeded"))
				}
				select {
				case err := <-copyErrors:
					cancelErr = errors.Join(cancelErr, err)
				default:
				}
				_ = s.Send(Message{Type: "ack", InstanceID: started.InstanceID, ExecID: started.ExecID})
				return result, errors.Join(result.Err(), cancelErr)
			}
		}
	}
}
