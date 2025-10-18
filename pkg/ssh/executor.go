package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

var (
	// ErrExecutionFailed is returned when command execution fails
	ErrExecutionFailed = errors.New("command execution failed")
)

// Executor provides high-level command execution capabilities with automatic
// session management and I/O handling.
type Executor struct {
	client *Client
}

// NewExecutor creates a new command executor using the given client
func NewExecutor(client *Client) *Executor {
	return &Executor{
		client: client,
	}
}

// ExecOptions configures command execution behavior
type ExecOptions struct {
	// I/O configuration
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Terminal configuration
	EnablePTY      bool
	TerminalWidth  int // Terminal width (0 for auto-detection)
	TerminalHeight int // Terminal height (0 for auto-detection)

	// Signal to send on context cancellation
	CancelSignal ssh.Signal
}

// DefaultExecOptions returns default execution options with stdout/stderr
// connected to os.Stdout/os.Stderr. Terminal size is set to 0 for auto-detection.
func DefaultExecOptions() *ExecOptions {
	return &ExecOptions{
		Stdin:          nil,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		EnablePTY:      false,
		TerminalWidth:  0, // Auto-detect
		TerminalHeight: 0, // Auto-detect
		CancelSignal:   ssh.SIGTERM,
	}
}

// WithPTY enables PTY allocation for the command.
// Use 0 for width or height to auto-detect from the current terminal.
//
// Examples:
//
//	opts.WithPTY(80, 24)  // Fixed size
//	opts.WithPTY(0, 0)    // Auto-detect both
//	opts.WithPTY(120, 0)  // Fixed width, auto-detect height
func (o *ExecOptions) WithPTY(width, height int) *ExecOptions {
	o.EnablePTY = true
	o.TerminalWidth = width
	o.TerminalHeight = height
	return o
}

// WithStdin sets the input stream
func (o *ExecOptions) WithStdin(r io.Reader) *ExecOptions {
	o.Stdin = r
	return o
}

// WithStdout sets the output stream
func (o *ExecOptions) WithStdout(w io.Writer) *ExecOptions {
	o.Stdout = w
	return o
}

// WithStderr sets the error stream
func (o *ExecOptions) WithStderr(w io.Writer) *ExecOptions {
	o.Stderr = w
	return o
}

// WithCancelSignal sets the signal to send on context cancellation
func (o *ExecOptions) WithCancelSignal(signal ssh.Signal) *ExecOptions {
	o.CancelSignal = signal
	return o
}

// Exec executes a command on the remote host and waits for it to complete.
// This is a high-level convenience method that handles session creation,
// I/O setup, and cleanup automatically.
//
// Example:
//
//	opts := ssh.DefaultExecOptions()
//	err := executor.Exec(ctx, opts, "ls", "-la", "/tmp")
//	if err != nil {
//	    return err
//	}
func (e *Executor) Exec(ctx context.Context, opts *ExecOptions, command ...string) error {
	// Create session
	session, err := e.client.NewSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	// Set up I/O streams
	if err := e.setupIO(session, opts); err != nil {
		return err
	}

	// Request PTY if needed
	if opts.EnablePTY {
		if err := session.RequestPTY(ctx, "", opts.TerminalWidth, opts.TerminalHeight); err != nil {
			return err
		}
	}

	// Set up context cancellation handling
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor context and send signal on cancellation
	go func() {
		<-cancelCtx.Done()
		if ctx.Err() != nil {
			logrus.Debugf("Context canceled, sending signal %s", opts.CancelSignal)
			if err := session.Signal(opts.CancelSignal); err != nil {
				logrus.Debugf("Failed to send signal: %v", err)
			}
		}
	}()

	// Execute command
	return session.Run(ctx, command...)
}

// ExecWithOutput executes a command and captures its output.
// This is a convenience method that returns stdout and stderr as byte slices.
//
// Example:
//
//	stdout, stderr, err := executor.ExecWithOutput(ctx, "ls", "-la")
//	if err != nil {
//	    return err
//	}
//	fmt.Println(string(stdout))
func (e *Executor) ExecWithOutput(ctx context.Context, command ...string) (stdout, stderr []byte, err error) {
	// Create buffers for output
	var stdoutBuf, stderrBuf syncBuffer

	// Configure options
	opts := DefaultExecOptions().
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf)

	// Execute command
	if err := e.Exec(ctx, opts, command...); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

// ExecInteractive executes a command in interactive mode with PTY.
// This automatically configures stdin/stdout/stderr for interactive use.
// Terminal size is auto-detected from the current terminal.
//
// Example:
//
//	err := executor.ExecInteractive(ctx, "/bin/bash")
//	if err != nil {
//	    return err
//	}
func (e *Executor) ExecInteractive(ctx context.Context, command ...string) error {
	opts := DefaultExecOptions().
		WithStdin(os.Stdin).
		WithPTY(0, 0) // Auto-detect terminal size

	return e.Exec(ctx, opts, command...)
}

// setupIO configures the session I/O streams based on options
// PTY mode uses direct assignment, non-PTY mode uses pipes with async copying
func (e *Executor) setupIO(session *Session, opts *ExecOptions) error {
	if opts.EnablePTY {
		// PTY mode: use direct assignment
		if opts.Stdin != nil {
			if err := session.SetStdin(opts.Stdin); err != nil {
				return fmt.Errorf("failed to configure stdin: %w", err)
			}
		}

		if opts.Stdout != nil {
			if err := session.SetStdout(opts.Stdout); err != nil {
				return fmt.Errorf("failed to configure stdout: %w", err)
			}
		}

		if opts.Stderr != nil {
			if err := session.SetStderr(opts.Stderr); err != nil {
				return fmt.Errorf("failed to configure stderr: %w", err)
			}
		}
	} else {
		// Non-PTY mode: use pipes with async copying
		if err := session.SetupPipes(opts.Stdin, opts.Stdout, opts.Stderr); err != nil {
			return fmt.Errorf("failed to setup pipes: %w", err)
		}
	}

	return nil
}

// Stream represents a streaming command execution
type Stream struct {
	session *Session
	cancel  context.CancelFunc
	done    chan error
}

// ExecStream starts a command in streaming mode without waiting for completion.
// The returned Stream can be used to monitor progress and wait for completion.
//
// Example:
//
//	stream, err := executor.ExecStream(ctx, opts, "tail", "-f", "/var/log/syslog")
//	if err != nil {
//	    return err
//	}
//	defer stream.Close()
//
//	// Do other work...
//
//	// Wait for completion
//	if err := stream.Wait(); err != nil {
//	    return err
//	}
func (e *Executor) ExecStream(ctx context.Context, opts *ExecOptions, command ...string) (*Stream, error) {
	// Create session
	session, err := e.client.NewSession(ctx)
	if err != nil {
		return nil, err
	}

	// Set up I/O streams
	if err := e.setupIO(session, opts); err != nil {
		session.Close()
		return nil, err
	}

	// Request PTY if needed
	if opts.EnablePTY {
		if err := session.RequestPTY(ctx, "", opts.TerminalWidth, opts.TerminalHeight); err != nil {
			session.Close()
			return nil, err
		}
	}

	// Create stream
	streamCtx, cancel := context.WithCancel(ctx)
	stream := &Stream{
		session: session,
		cancel:  cancel,
		done:    make(chan error, 1),
	}

	// Set up context cancellation handling
	go func() {
		<-streamCtx.Done()
		if ctx.Err() != nil {
			logrus.Debugf("Stream context canceled, sending signal %s", opts.CancelSignal)
			if err := session.Signal(opts.CancelSignal); err != nil {
				logrus.Debugf("Failed to send signal: %v", err)
			}
		}
	}()

	// Start command in background
	if err := session.Start(ctx, command...); err != nil {
		cancel()
		session.Close()
		return nil, err
	}

	// Wait for completion in background
	go func() {
		stream.done <- session.Wait()
	}()

	return stream, nil
}

// Wait waits for the streaming command to complete
func (s *Stream) Wait() error {
	return <-s.done
}

// Close closes the stream and releases resources
func (s *Stream) Close() error {
	s.cancel()
	return s.session.Close()
}

// syncBuffer is a thread-safe buffer for capturing output
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]byte, len(b.buf))
	copy(result, b.buf)
	return result
}
