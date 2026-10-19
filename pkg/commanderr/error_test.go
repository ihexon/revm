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
