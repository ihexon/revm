package commanderr

import (
	"errors"
	"linuxvm/pkg/execution"
	"testing"
)

func TestDiagnosticAndStatusPrecedence(t *testing.T) {
	cause := &execution.Canceled{Signal: "SIGPIPE"}
	code := 143
	err := errors.Join(cause, &execution.ExitError{Result: execution.Result{Code: &code}}, cause)
	if ExitCode(err) != 141 {
		t.Fatal("cleanup signal replaced original cancellation")
	}
	if got := Diagnostic(errors.Join(errors.New("failure"), errors.New("failure\ncleanup failed"))); got != "failure; cleanup failed" {
		t.Fatal(got)
	}
}

func TestGuestExitDoesNotPolluteStderr(t *testing.T) {
	code := 42
	guest := execution.Result{Code: &code, Cleanup: "complete", Output: "complete"}
	if got := Diagnostic(errors.Join(guest.Err())); got != "" {
		t.Fatal(got)
	}
	if ExitCode(guest.Err()) != 42 {
		t.Fatal("lost exit status")
	}
	if got := Diagnostic(errors.Join(guest.Err(), errors.New("disk cleanup failed"))); got == "" {
		t.Fatal("hidden cleanup failure")
	}
	guest.Kind, guest.Message = "start", "command not found"
	if got := Diagnostic(guest.Err()); got != "command not found" {
		t.Fatal(got)
	}
}
