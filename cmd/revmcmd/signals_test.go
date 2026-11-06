package revmcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/commanderr"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCLIBrokenPipe(t *testing.T) {
	if os.Getenv("REVM_TEST_CLI_PIPE") == "1" {
		// Match the executable's process-wide policy; Execute must not
		// re-subscribe to SIGPIPE and turn internal I/O failures into exits.
		signal.Ignore(syscall.SIGPIPE)
		go func() {
			// The parent releases us after observing CLI output, so Execute
			// has already installed its signal handlers.
			var ready [1]byte
			if _, err := io.ReadFull(os.Stdin, ready[:]); err != nil {
				os.Exit(90)
			}
			fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
			if err != nil {
				os.Exit(91)
			}
			unix.Close(fds[1])
			_, err = unix.Write(fds[0], []byte("probe"))
			unix.Close(fds[0])
			if !errors.Is(err, syscall.EPIPE) {
				os.Exit(92)
			}
			fmt.Fprint(os.Stderr, "broken pipe observed")
		}()
		err := Execute(context.Background(), []string{"revm", "logs", "pipe-test"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(commanderr.ExitCode(err))
	}
	for _, brokenOutput := range []bool{false, true} {
		name := "internal socket preserves CLI and host cancellation"
		if brokenOutput {
			name = "closed stdout cancels CLI"
		}
		t.Run(name, func(t *testing.T) { checkCLIBrokenPipe(t, brokenOutput) })
	}
}

func checkCLIBrokenPipe(t *testing.T, brokenOutput bool) {
	t.Helper()
	root := t.TempDir()
	session := filepath.Join(root, "state", "sessions", "pipe-test")
	logs := filepath.Join(session, "runs", "11111111-1111-4111-8111-111111111111", "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session, "session.json"), []byte(`{"version":1,"id":"pipe-test","instanceID":"11111111-1111-4111-8111-111111111111","config":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(logs), "result.json"), []byte(`{"version":1,"sessionID":"pipe-test","instanceID":"11111111-1111-4111-8111-111111111111"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "revm.log"), bytes.Repeat([]byte("x"), 1<<20), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIBrokenPipe$")
	cmd.Env = append(os.Environ(), "REVM_TEST_CLI_PIPE=1", "REVM_HOME="+root)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	// Own the read ends so cmd.Wait cannot close them before diagnostics are read.
	output, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	defer outWriter.Close()
	cmd.Stdout = outWriter
	diagnostic, errWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer diagnostic.Close()
	defer errWriter.Close()
	cmd.Stderr = errWriter
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	outWriter.Close()
	errWriter.Close()
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	defer func() { cancel(); <-done }()
	var first [1]byte
	if _, err := io.ReadFull(output, first[:]); err != nil {
		details, _ := io.ReadAll(diagnostic)
		t.Fatalf("read first log byte: %v; stderr: %s", err, details)
	}
	details := make(chan []byte, 1)
	if brokenOutput {
		go func() { b, _ := io.ReadAll(diagnostic); details <- b }()
		output.Close()
	} else {
		if _, err := input.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		marker := make([]byte, len("broken pipe observed"))
		if _, err := io.ReadFull(diagnostic, marker); err != nil {
			t.Fatal(err)
		}
		if string(marker) != "broken pipe observed" {
			t.Fatalf("unexpected diagnostic: %q", marker)
		}
		select {
		case <-done:
			t.Fatalf("internal socket EPIPE canceled CLI: %v", waitErr)
		case <-time.After(100 * time.Millisecond):
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
	}
	<-done
	var exit *exec.ExitError
	want := 143
	if brokenOutput {
		want = 141
	}
	if !errors.As(waitErr, &exit) || exit.ExitCode() != want {
		if brokenOutput {
			t.Fatalf("want exit %d, got %v; stderr: %s", want, waitErr, <-details)
		}
		t.Fatalf("want exit %d, got %v", want, waitErr)
	}
}
