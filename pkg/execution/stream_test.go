package execution

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func runLocal(t *testing.T, argv []string, input io.Reader, tty bool) (Result, []byte, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := net.Pipe()
	go Serve(ctx, b, "test-instance", &Registry{})
	var out, diag bytes.Buffer
	r, err := Execute(ctx, a, Request{Version: 2, InstanceID: "test-instance", RequestID: "request", Argv: argv, TTY: tty}, Options{IO: IO{Stdin: input, Stdout: &out, Stderr: &diag}, StopTimeout: 100 * time.Millisecond})
	return r, out.Bytes(), diag.Bytes(), err
}

func TestStreamingRoundTrip(t *testing.T) {
	input := bytes.Repeat([]byte{0, 255, 1, '\n', 13}, 400000)
	r, out, diag, err := runLocal(t, []string{"sh", "-c", "cat; printf error >&2; exit 42"}, bytes.NewReader(input), false)
	if r.ExitCode() != 42 || err == nil || !bytes.Equal(input, out) || string(diag) != "error" {
		t.Fatalf("result=%+v error=%v bytes=%d stderr=%q", r, err, len(out), diag)
	}
}

func TestStreamingBeforeEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	go Serve(ctx, b, "instance", &Registry{})
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	output, read := io.Pipe()
	defer output.Close()
	defer read.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Execute(ctx, a, Request{Version: 2, InstanceID: "instance", RequestID: "live", Argv: []string{"cat"}}, Options{IO: IO{Stdin: input, Stdout: read}, StopTimeout: time.Second})
		done <- err
	}()
	go func() { _, _ = writer.Write([]byte("before EOF")) }()
	got := make([]byte, 10)
	if _, err := io.ReadFull(output, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "before EOF" {
		t.Fatalf("%q", got)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPTYExecArgv(t *testing.T) {
	r, out, _, err := runLocal(t, []string{"sh", "-c", "test -t 0 && printf pty-command"}, bytes.NewReader(nil), true)
	if err != nil || r.ExitCode() != 0 || !bytes.Contains(out, []byte("pty-command")) {
		t.Fatalf("%+v %v %q", r, err, out)
	}
}
