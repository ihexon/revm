package system

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestNonTerminalHasNoRestoreState(t *testing.T) {
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if SaveTerminal(int(file.Fd())) != nil {
		t.Fatal("non-terminal must not produce a zero-valued restoration state")
	}
}

func TestClosedTerminalCannotRestart(t *testing.T) {
	terminal := &CommandTerminal{}
	terminal.Close()
	if err := terminal.Start(true, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("late execution response can reopen terminal: %v", err)
	}
	terminal.Event("SIGCONT")
}

func TestCommandRawForwardsControlCharacters(t *testing.T) {
	for _, test := range []struct {
		name string
		key  byte
		cc   int
	}{
		{"Ctrl-C", 0x03, unix.VINTR},
		{"Ctrl-Z", 0x1a, unix.VSUSP},
		{"Ctrl-Backslash", 0x1c, unix.VQUIT},
	} {
		t.Run(test.name, func(t *testing.T) {
			master, slave, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			defer slave.Close()
			fd := int(slave.Fd())
			initial := SaveTerminal(fd)
			if initial == nil {
				t.Fatal("read terminal state")
			}
			initial.Lflag |= unix.ISIG | unix.ICANON | unix.ECHO
			initial.Cc[test.cc] = test.key
			if err := RestoreTerminal(fd, initial); err != nil {
				t.Fatal(err)
			}
			state, err := makeCommandRaw(fd)
			if err != nil {
				t.Fatal(err)
			}
			defer term.Restore(fd, state)

			// A real terminal must deliver the control byte to the stdin copier,
			// rather than consume it as a local signal. The marker also ensures
			// a swallowed control byte fails without leaving a blocked read.
			if _, err := master.Write([]byte{test.key, 'x'}); err != nil {
				t.Fatal(err)
			}
			for _, want := range []byte{test.key, 'x'} {
				fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
				var n int
				for {
					n, err = unix.Poll(fds, 1000)
					if !errors.Is(err, unix.EINTR) {
						break
					}
				}
				if err != nil || n == 0 {
					t.Fatalf("control input was not delivered: poll=%d err=%v", n, err)
				}
				var got [1]byte
				if n, err := slave.Read(got[:]); err != nil || n != 1 || got[0] != want {
					t.Fatalf("read %x (%d bytes, %v), want %02x", got, n, err, want)
				}
			}
			if err := term.Restore(fd, state); err != nil {
				t.Fatal(err)
			}
			restored := SaveTerminal(fd)
			if restored == nil || restored.Lflag&(unix.ISIG|unix.ICANON|unix.ECHO) != initial.Lflag&(unix.ISIG|unix.ICANON|unix.ECHO) {
				t.Fatal("original terminal signal, canonical and echo modes were not restored")
			}
		})
	}
}
