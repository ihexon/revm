package revmcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/revm"
	"strings"
	"testing"
	"time"
)

func TestSessionTableShowsCommandTimeAndExit(t *testing.T) {
	start := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Second)
	code := 42
	view := revm.SessionView{Session: revm.Session{ID: "failed-run", Config: revm.Config{RunMode: revm.ModeRootfs, Exec: execution.Request{Argv: []string{"sh", "-c", "echo hello\nexit 42"}}}}, Status: "stopped", LastRun: &revm.RunRecord{StartedAt: start, FinishedAt: &end, ExitCode: &code}}
	var out bytes.Buffer
	if err := writeSessions(&out, []revm.SessionView{view}, end); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"failed-run", "run", "2026-10-19T10:00:00Z", "3s", "42", `"echo hello\nexit 42"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
	if strings.Count(out.String(), "\n") != 2 {
		t.Fatal("command split the table", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestQueryOutputFailuresAreReturned(t *testing.T) {
	if err := writeSessions(failingWriter{}, nil, time.Now()); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if err := writeVolumes(failingWriter{}, nil); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
func TestInvalidLogTailIsUsageError(t *testing.T) {
	app := NewApp("revm")
	app.Writer = io.Discard
	err := app.Run(context.Background(), []string{"revm", "logs", "--tail=-2", "missing"})
	if commanderr.ExitCode(err) != 2 || !strings.Contains(err.Error(), "--tail") {
		t.Fatal(err)
	}
}
