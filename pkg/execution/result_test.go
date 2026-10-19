package execution

import (
	"os/exec"
	"testing"
)

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
