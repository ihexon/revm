// Package commanderr defines errors understood at the CLI boundary. Libraries
// return errors; only the executable decides whether to terminate the process.
package commanderr

import (
	"errors"
	"linuxvm/pkg/execution"
	"strings"
)

type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func Usage(err error) error {
	if err == nil {
		return nil
	}
	return &UsageError{Err: err}
}

// ExitCode reserves 2 for invalid invocation and 1 for operational failures.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var canceled *execution.Canceled
	if errors.As(err, &canceled) {
		return canceled.ExitCode()
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return 2
	}
	var guest *execution.ExitError
	if errors.As(err, &guest) {
		return guest.Result.ExitCode()
	}
	return 1
}

// Diagnostic keeps joined errors readable without repeating the same cause.
// Exit codes and typed results are the stable programmatic interface.
func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	seen := map[string]bool{}
	var messages []string
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			seen[line] = true
			messages = append(messages, line)
		}
	}
	return strings.Join(messages, "; ")
}
