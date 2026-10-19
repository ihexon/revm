// Package execution is the shared, transport-independent command result contract.
package execution

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

type Result struct {
	Code    *int   `json:"code,omitempty"`
	Signal  string `json:"signal,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
	Cleanup string `json:"cleanup"`
	Output  string `json:"output"`
}

type ExitError struct{ Result Result }

func (e *ExitError) Error() string {
	if e.Result.Message != "" {
		return e.Result.Message
	}
	if e.Result.Signal != "" {
		return "guest command terminated by " + e.Result.Signal
	}
	if e.Result.Code != nil {
		return fmt.Sprintf("guest command exited with code %d", *e.Result.Code)
	}
	return "guest command result is unknown"
}

func (r Result) ExitCode() int {
	if r.Signal != "" {
		if n := LinuxSignalNumber(r.Signal); n != 0 {
			return 128 + n
		}
		return 1
	}
	if r.Code != nil && *r.Code != 0 {
		return *r.Code
	}
	if r.Code == nil || r.Kind != "" || r.Cleanup == "failed" || r.Output == "truncated" {
		return 1
	}
	return 0
}
func (r Result) Err() error {
	if r.ExitCode() == 0 {
		return nil
	}
	return &ExitError{Result: r}
}

func FromProcess(err error) Result {
	code := 0
	r := Result{Code: &code, Cleanup: "complete", Output: "complete"}
	if err == nil {
		return r
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			r.Code = nil
			r.Signal = SignalName(ws.Signal())
			return r
		}
		code = ee.ExitCode()
		return r
	}
	r.Kind, r.Message = "start", err.Error()
	code = 1
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		code = 127
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.ENOEXEC) || errors.Is(err, syscall.EISDIR) {
		code = 126
	}
	return r
}

// Wire signals are names: Darwin and Linux numeric assignments differ.
func SignalName(sig syscall.Signal) string {
	for name, value := range signals {
		if value == sig {
			return name
		}
	}
	return "UNKNOWN"
}
func Signal(name string) (syscall.Signal, bool) { s, ok := signals[name]; return s, ok }

var signals = map[string]syscall.Signal{
	"SIGHUP": syscall.SIGHUP, "SIGINT": syscall.SIGINT, "SIGQUIT": syscall.SIGQUIT,
	"SIGILL": syscall.SIGILL, "SIGTRAP": syscall.SIGTRAP, "SIGABRT": syscall.SIGABRT,
	"SIGBUS": syscall.SIGBUS, "SIGFPE": syscall.SIGFPE, "SIGKILL": syscall.SIGKILL,
	"SIGUSR1": syscall.SIGUSR1, "SIGSEGV": syscall.SIGSEGV, "SIGUSR2": syscall.SIGUSR2,
	"SIGPIPE": syscall.SIGPIPE, "SIGALRM": syscall.SIGALRM, "SIGTERM": syscall.SIGTERM,
	"SIGCHLD": syscall.SIGCHLD, "SIGCONT": syscall.SIGCONT, "SIGSTOP": syscall.SIGSTOP,
	"SIGTSTP": syscall.SIGTSTP, "SIGTTIN": syscall.SIGTTIN, "SIGTTOU": syscall.SIGTTOU,
	"SIGURG": syscall.SIGURG, "SIGXCPU": syscall.SIGXCPU, "SIGXFSZ": syscall.SIGXFSZ,
	"SIGVTALRM": syscall.SIGVTALRM, "SIGPROF": syscall.SIGPROF, "SIGWINCH": syscall.SIGWINCH,
	"SIGIO": syscall.SIGIO, "SIGSYS": syscall.SIGSYS,
}

func LinuxSignalNumber(name string) int {
	return map[string]int{"SIGHUP": 1, "SIGINT": 2, "SIGQUIT": 3, "SIGILL": 4, "SIGTRAP": 5, "SIGABRT": 6, "SIGBUS": 7, "SIGFPE": 8, "SIGKILL": 9, "SIGUSR1": 10, "SIGSEGV": 11, "SIGUSR2": 12, "SIGPIPE": 13, "SIGALRM": 14, "SIGTERM": 15, "SIGCHLD": 17, "SIGCONT": 18, "SIGSTOP": 19, "SIGTSTP": 20, "SIGTTIN": 21, "SIGTTOU": 22, "SIGURG": 23, "SIGXCPU": 24, "SIGXFSZ": 25, "SIGVTALRM": 26, "SIGPROF": 27, "SIGWINCH": 28, "SIGIO": 29, "SIGSYS": 31}[name]
}
