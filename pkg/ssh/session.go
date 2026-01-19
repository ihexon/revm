package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/system"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var (
	// ErrSessionClosed is returned when operations are attempted on a closed session
	ErrSessionClosed = errors.New("SSH session is closed")
	// ErrPTYRequestFailed is returned when PTY allocation fails
	ErrPTYRequestFailed = errors.New("failed to request PTY")
	// ErrCommandFailed is returned when command execution fails
	ErrCommandFailed = errors.New("command execution failed")
)

// Session represents an SSH session with automatic resource management
type Session struct {
	session *ssh.Session
	client  *Client

	// I/O streams
	stdin  io.WriteCloser
	stdout io.Reader
	stderr io.Reader

	// I/O wait group for non-PTY mode
	ioWg sync.WaitGroup

	// PTY state
	ptyAllocated          bool
	previousTerminalState *system.TerminalState

	// Lifecycle
	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.RWMutex
}

// RequestPTY allocates a pseudo-terminal for the session with specified parameters.
// This should be called before starting command execution if interactive terminal is needed.
//
// Parameters:
//   - termType: Terminal type (e.g., "xterm-256color"). Use empty string for auto-detection from $TERM.
//   - width: Terminal width in columns. Use 0 for auto-detection from current terminal.
//   - height: Terminal height in rows. Use 0 for auto-detection from current terminal.
//
// If width or height is 0, the method will attempt to get the size from the current terminal.
// If termType is empty, it defaults to the value of $TERM or "xterm-256color".
//
// Example:
//
//	// Auto-detect everything
//	if err := session.RequestPTY(ctx, "", 0, 0); err != nil {
//	    return err
//	}
//
//	// Specify custom size
//	if err := session.RequestPTY(ctx, "xterm-256color", 120, 40); err != nil {
//	    return err
//	}
func (s *Session) RequestPTY(ctx context.Context, termType string, width, height int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	if s.ptyAllocated {
		return errors.New("PTY already allocated for this session")
	}

	// Configure terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // Enable echoing
		ssh.IUTF8:         1,     // UTF-8 input
		ssh.TTY_OP_ISPEED: 14400, // Input speed
		ssh.TTY_OP_OSPEED: 14400, // Output speed
	}

	// Auto-detect terminal type if not specified
	if termType == "" {
		termType = system.GetTerminalType()
	}

	// Auto-detect terminal size if not specified
	if width == 0 || height == 0 {
		autoWidth, autoHeight, err := system.GetTerminalSize()
		if err != nil {
			return fmt.Errorf("failed to get terminal size: %w", err)
		}
		if width == 0 {
			width = autoWidth
		}
		if height == 0 {
			height = autoHeight
		}
	}

	// Request PTY from SSH server
	if err := s.session.RequestPty(termType, height, width, modes); err != nil {
		return fmt.Errorf("%w: %v", ErrPTYRequestFailed, err)
	}

	s.ptyAllocated = true

	// If stdin is a terminal, set it to raw mode and monitor resize
	if system.IsTerminal() {
		previousState, err := system.MakeStdinRaw()
		if err != nil {
			return err
		}
		s.previousTerminalState = previousState

		resizeFn := func() {
			s.mu.RLock()
			session := s.session
			s.mu.RUnlock()

			if session != nil {
				newWidth, newHeight, err := term.GetSize(int(os.Stdin.Fd()))
				if err != nil {
					logrus.Infof("Failed to get terminal size: %v", err)
					return
				}

				if err := session.WindowChange(newHeight, newWidth); err != nil {
					logrus.Infof("Failed to change window size: %v", err)
				} else {
					logrus.Infof("Terminal resized to %dx%d", newWidth, newHeight)
				}
			}
		}

		system.OnTerminalResize(resizeFn, ctx)
	}

	logrus.Infof("PTY allocated: %s (%dx%d)", termType, width, height)
	return nil
}

// SetStdin configures the input stream for the session (for PTY mode)
// This directly assigns the reader to session.stdin
func (s *Session) SetStdin(r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	s.session.Stdin = r
	return nil
}

// SetStdout configures the output stream for the session (for PTY mode)
// This directly assigns the writer to session.stdout
func (s *Session) SetStdout(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	s.session.Stdout = w
	return nil
}

// SetStderr configures the error stream for the session (for PTY mode)
// This directly assigns the writer to session.stderr
func (s *Session) SetStderr(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	s.session.Stderr = w
	return nil
}

// SetupPipes configures I/O pipes for non-PTY mode with async copying
// This should be used instead of SetStdin/SetStdout/SetStderr for non-PTY sessions
func (s *Session) SetupPipes(stdin io.Reader, stdout, stderr io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	// Set up stdout pipe
	if stdout != nil {
		stdoutPipe, err := s.session.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdout pipe: %w", err)
		}
		s.stdout = stdoutPipe
		s.ioWg.Add(1)
		go func() {
			defer s.ioWg.Done()
			_, err := io.Copy(stdout, stdoutPipe)
			logrus.Infof("stdout pipe copy finished with error: %v", err)
		}()
	}

	// Set up stderr pipe
	if stderr != nil {
		stderrPipe, err := s.session.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed to get stderr pipe: %w", err)
		}
		s.stderr = stderrPipe
		s.ioWg.Add(1)
		go func() {
			defer s.ioWg.Done()
			_, err := io.Copy(stderr, stderrPipe)
			logrus.Infof("stderr pipe copy finished with error: %v", err)
		}()
	}

	// Set up stdin if provided
	if stdin != nil {
		stdinPipe, err := s.session.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to get stdin pipe: %w", err)
		}
		s.stdin = stdinPipe
		s.ioWg.Add(1)
		go func() {
			defer s.ioWg.Done()
			_, _ = io.Copy(stdinPipe, stdin)
			stdinPipe.Close()
			logrus.Infof("stdin pipe copy finished")
		}()
	}

	return nil
}

// Start begins execution of the given command without waiting for it to complete.
// Use Wait() to wait for the command to finish.
//
// Example:
//
//	if err := session.Start(ctx, "ls", "-la"); err != nil {
//	    return err
//	}
//	if err := session.Wait(); err != nil {
//	    return err
//	}
func (s *Session) Start(ctx context.Context, command ...string) error {
	if len(command) == 0 || command[0] == "" {
		return errors.New("command is empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	cmdString := (&SessionConfig{Command: command}).CommandString()
	logrus.Infof("Starting command: %s", cmdString)

	if err := s.session.Start(cmdString); err != nil {
		return fmt.Errorf("%w: %v", ErrCommandFailed, err)
	}

	return nil
}

// Run executes the given command and waits for it to complete.
// This is equivalent to calling Start() followed by Wait().
//
// Example:
//
//	if err := session.Run(ctx, "echo", "hello"); err != nil {
//	    return err
//	}
func (s *Session) Run(ctx context.Context, command ...string) error {
	if err := s.Start(ctx, command...); err != nil {
		return err
	}
	return s.Wait()
}

// Wait waits for the remote command to exit and returns its exit status.
// The returned error will be nil if the command runs, has no problems
// copying stdin, stdout, and stderr, and exits with a zero exit status.
// For non-PTY sessions with pipes, this also waits for all I/O to complete.
func (s *Session) Wait() error {
	s.mu.RLock()
	session := s.session
	s.mu.RUnlock()

	if session == nil {
		return ErrSessionClosed
	}

	// Wait for the command to exit
	err := session.Wait()

	// Wait for all I/O goroutines to finish (non-PTY mode)
	// This ensures all output is copied before returning
	s.ioWg.Wait()

	logrus.Infof("All I/O operations completed")

	if err != nil {
		// Check if it's an exit error
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("%w: command exited with code %d", ErrCommandFailed, exitErr.ExitStatus())
		}
		return fmt.Errorf("%w: %v", ErrCommandFailed, err)
	}

	return nil
}

// Signal sends a signal to the remote process.
// This is typically used to interrupt or terminate a running command.
//
// Common signals:
//   - ssh.SIGTERM: Request graceful termination
//   - ssh.SIGKILL: Force immediate termination
//   - ssh.SIGINT: Interrupt (Ctrl+C)
func (s *Session) Signal(signal ssh.Signal) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	if err := s.session.Signal(signal); err != nil {
		return fmt.Errorf("failed to send signal %s: %w", signal, err)
	}

	logrus.Infof("Sent signal %s to remote process", signal)
	return nil
}

// WindowChange informs the remote terminal of a size change
func (s *Session) WindowChange(height, width int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.isClosed() {
		return ErrSessionClosed
	}

	if !s.ptyAllocated {
		return errors.New("cannot change window size without PTY")
	}

	if err := s.session.WindowChange(height, width); err != nil {
		return fmt.Errorf("failed to change window size: %w", err)
	}

	return nil
}

// Close closes the session and releases all resources.
// It also restores the terminal to its original state if it was set to raw mode.
// It is safe to call Close multiple times.
func (s *Session) Close() error {
	var finalErr error

	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Signal closure
		close(s.closed)

		// Close stdin pipe if we opened it
		if s.stdin != nil {
			if err := s.stdin.Close(); err != nil {
				logrus.Infof("Failed to close stdin pipe: %v", err)
			}
			s.stdin = nil
		}

		// Close session (this will close pipes, causing I/O goroutines to exit)
		if s.session != nil {
			if err := s.session.Close(); err != nil {
				finalErr = fmt.Errorf("failed to close SSH session: %w", err)
			}
			s.session = nil
		}

		logrus.Infof("SSH session closed")
	})

	// Wait for all I/O goroutines to finish after session is closed
	// This ensures no goroutines are leaked and all I/O is complete
	s.ioWg.Wait()
	logrus.Infof("All I/O goroutines finished")

	// Restore terminal state using system package
	if s.previousTerminalState != nil {
		system.ResetStdin(s.previousTerminalState)
		s.previousTerminalState = nil
	}

	return finalErr
}

// isClosed returns true if the session has been closed
func (s *Session) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}
