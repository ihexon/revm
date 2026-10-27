//go:build linux || darwin

package system

import (
	"context"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func ForegroundTerminal(fd int) bool {
	if !term.IsTerminal(fd) {
		return false
	}
	pgid, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	return err == nil && pgid == unix.Getpgrp()
}

type CommandTerminal struct {
	mu      sync.Mutex
	state   *term.State
	enabled bool
	resize  func()
	closed  bool
}

// ProcessTerminal has the same lifetime as the CLI. Main closes it before
// exiting so a late execution response cannot put a restored terminal in raw mode.
var ProcessTerminal CommandTerminal

func (t *CommandTerminal) Start(enabled bool, resize func()) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return context.Canceled
	}
	t.enabled = enabled
	t.resize = resize
	if !enabled || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	if !ForegroundTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("cannot take over a background terminal; bring revm to the foreground or use --tty=never")
	}
	var err error
	t.state, err = makeCommandRaw(int(os.Stdin.Fd()))
	return err
}
func (t *CommandTerminal) Restore() { t.mu.Lock(); defer t.mu.Unlock(); t.restore() }
func (t *CommandTerminal) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.restore()
}
func (t *CommandTerminal) restore() {
	if t.state != nil {
		_ = term.Restore(int(os.Stdin.Fd()), t.state)
		t.state = nil
	}
}
func (t *CommandTerminal) Event(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	if name == "SIGTSTP" {
		t.restore()
		return
	}
	if name == "SIGCONT" && t.enabled && ForegroundTerminal(int(os.Stdin.Fd())) && t.state == nil {
		t.state, _ = makeCommandRaw(int(os.Stdin.Fd()))
	}
	if (name == "SIGCONT" || name == "SIGWINCH") && t.resize != nil {
		t.resize()
	}
}

// Keep terminal-generated INT/QUIT/TSTP local to the CLI. Cancellation ends
// the VM process; control bytes are not reinterpreted in the guest stream.
func makeCommandRaw(fd int) (*term.State, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	raw := SaveTerminal(fd)
	if raw == nil {
		_ = term.Restore(fd, state)
		return nil, fmt.Errorf("read raw terminal state")
	}
	raw.Lflag |= unix.ISIG
	if err = RestoreTerminal(fd, raw); err != nil {
		_ = term.Restore(fd, state)
		return nil, err
	}
	return state, nil
}
