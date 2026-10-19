package execution

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestExecFailuresUseShellStatus(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
	}{{os.ErrNotExist, 127}, {os.ErrPermission, 126}, {syscall.ENOEXEC, 126}, {syscall.EISDIR, 126}} {
		result := FromProcess(fmt.Errorf("start: %w", tc.err))
		if result.ExitCode() != tc.code || result.Kind != "start" {
			t.Fatalf("%v: %+v", tc.err, result)
		}
	}
}

func TestProcessResults(t *testing.T) {
	for _, tc := range []struct {
		command string
		code    int
		signal  string
	}{
		{"exit 0", 0, ""}, {"exit 42", 42, ""}, {"kill -TERM $$", 143, "SIGTERM"},
		{"kill -USR1 $$", 138, "SIGUSR1"},
	} {
		r := FromProcess(exec.Command("sh", "-c", tc.command).Run())
		if r.ExitCode() != tc.code || r.Signal != tc.signal {
			t.Fatalf("%s: %+v", tc.command, r)
		}
	}
	r := FromProcess(exec.Command("/no/such/revm-test-command").Run())
	if r.ExitCode() != 127 {
		t.Fatalf("not found: %+v", r)
	}
	if (Result{}).ExitCode() == 0 {
		t.Fatal("unknown result reported success")
	}
}

func TestRealtimeSignalStatus(t *testing.T) {
	if name := SignalName(syscall.Signal(34)); name != "SIG34" {
		t.Fatal(name)
	}
	if code := (Result{Signal: "SIG34"}).ExitCode(); code != 162 {
		t.Fatal(code)
	}
}
