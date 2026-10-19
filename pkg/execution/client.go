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
	outputCtx, stopOutput := context.WithCancel(context.Background())
	defer stopOutput()
	outputs, restoreOutputs, err := outputWriters(outputCtx, opts.Stdout, opts.Stderr)
	if err != nil {
		return Result{}, err
	}
	defer restoreOutputs()
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
	}{{outputs[0], s.Stdout}, {outputs[1], s.Stderr}} {
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
	defer func() { stopOutput(); s.Session.Close(); <-outputDone }()
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
	for {
		select {
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
				if m.Type == "exit" && opts.Controller != nil {
					opts.Controller.Complete()
				}
			}
			if m.Type == "completed" {
				stopInput()
				select {
				case <-outputDone:
				case <-deadline:
					return result, errors.Join(cancelErr, errors.New("output drain deadline exceeded"))
				case <-force:
					return result, errors.Join(cancelErr, errors.New("output drain interrupted"))
				case <-cancelCh:
					return result, errors.Join(context.Cause(ctx), errors.New("output drain interrupted"))
				case <-time.After(3 * time.Second):
					return result, errors.Join(cancelErr, errors.New("output drain deadline exceeded"))
				}
				select {
				case err := <-copyErrors:
					cancelErr = errors.Join(cancelErr, err)
				default:
				}
				_ = s.Send(Message{Type: "ack", InstanceID: started.InstanceID, ExecID: started.ExecID})
				_ = s.Stdin.Close()
				select {
				case <-inputDone:
				case <-time.After(200 * time.Millisecond):
				}
				return result, errors.Join(result.Err(), cancelErr)
			}
		}
	}
}
