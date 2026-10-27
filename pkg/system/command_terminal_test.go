package system

import (
	"context"
	"errors"
	"os"
	"testing"
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
