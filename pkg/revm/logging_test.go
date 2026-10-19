package revm

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupDiagnosticIncludesBoundedLogAndSession(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("earlier output\n", 500) + "fatal: cannot mount rootfs\n"
	if err := os.WriteFile(filepath.Join(dir, "vm.log"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	lease := &sessionRun{logDir: dir, session: Session{ID: "failed-run"}}
	cause := errors.New("VMM exited with code 1")
	err := lease.diagnose(cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "fatal: cannot mount rootfs") || !strings.Contains(err.Error(), "revm inspect failed-run") || len(err.Error()) > 2500 {
		t.Fatal(err)
	}
	if strings.Count(guestLogTail(filepath.Join(dir, "vm.log")), "\n") > 7 {
		t.Fatal("unbounded log tail")
	}
}

func TestLogTailAcrossBlocksAndPartialLines(t *testing.T) {
	for _, tc := range []struct {
		content string
		lines   int
		want    string
	}{
		{"", 10, ""}, {"one\ntwo\nthree\n", 1, "three\n"},
		{"one\ntwo\nthree", 2, "two\nthree"}, {"one\n\n", 1, "\n"},
		{"one\ntwo\n", 5, "one\ntwo\n"}, {"one\ntwo\n", 0, ""},
		{"one\ntwo\n", -1, "one\ntwo\n"},
		{strings.Repeat("x", 100000) + "\nlast\n", 1, "last\n"},
		{"first\n" + strings.Repeat("x", 100000), 1, strings.Repeat("x", 100000)},
	} {
		f, err := os.CreateTemp(t.TempDir(), "log")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.WriteString(tc.content); err != nil {
			t.Fatal(err)
		}
		if err = seekLogTail(f, tc.lines); err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(got) != tc.want {
			t.Fatalf("tail %d: got %d bytes, want %d; %v", tc.lines, len(got), len(tc.want), err)
		}
	}
}
