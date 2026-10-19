//go:build linux || darwin

package system

import (
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
}

func (t *CommandTerminal) Start(enabled bool, resize func()) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = enabled
	t.resize = resize
	if !enabled || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	if !ForegroundTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("cannot take over a background terminal; bring revm to the foreground or use --tty=never")
	}
	var err error
	t.state, err = term.MakeRaw(int(os.Stdin.Fd()))
	return err
}
func (t *CommandTerminal) Restore() { t.mu.Lock(); defer t.mu.Unlock(); t.restore() }
func (t *CommandTerminal) restore() {
	if t.state != nil {
		_ = term.Restore(int(os.Stdin.Fd()), t.state)
		t.state = nil
	}
}
func (t *CommandTerminal) Event(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if name == "SIGTSTP" {
		t.restore()
		return
	}
	if name == "SIGCONT" && t.enabled && ForegroundTerminal(int(os.Stdin.Fd())) && t.state == nil {
		t.state, _ = term.MakeRaw(int(os.Stdin.Fd()))
	}
	if (name == "SIGCONT" || name == "SIGWINCH") && t.resize != nil {
		t.resize()
	}
}
