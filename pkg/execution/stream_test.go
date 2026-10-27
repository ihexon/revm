package execution

import (
	"bytes"
	"context"
	"errors"
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
	go Serve(ctx, b, "test-instance")
	var out, diag bytes.Buffer
	r, err := Execute(ctx, a, Request{Version: ProtocolVersion, InstanceID: "test-instance", Argv: argv, TTY: tty}, Options{IO: IO{Stdin: input, Stdout: &out, Stderr: &diag}})
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
	go Serve(ctx, b, "instance")
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	output, read := io.Pipe()
	defer output.Close()
	defer read.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Execute(ctx, a, Request{Version: ProtocolVersion, InstanceID: "instance", Argv: []string{"cat"}}, Options{IO: IO{Stdin: input, Stdout: read}})
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

func TestDisconnectBeforeResultFails(t *testing.T) {
	a, b := net.Pipe()
	go func() {
		s, err := streams(context.Background(), b, true)
		if err != nil {
			return
		}
		defer s.Session.Close()
		var req Request
		if s.Decoder.Decode(&req) == nil {
			s.Send(Message{Type: "started"})
		}
	}()
	result, err := Execute(context.Background(), a, Request{Version: ProtocolVersion, InstanceID: "instance", Argv: []string{"true"}}, Options{})
	if err == nil || result.Code != nil || result.ExitCode() == 0 {
		t.Fatalf("disconnect reported a known exit: %+v %v", result, err)
	}
}

type failedInput struct{ err error }

func (r failedInput) Read([]byte) (int, error) { return 0, r.err }

func TestInputReadFailure(t *testing.T) {
	failure := errors.New("input device failed")
	_, _, _, err := runLocal(t, []string{"cat"}, failedInput{failure}, false)
	if !errors.Is(err, failure) {
		t.Fatalf("lost stdin error: %v", err)
	}
}

func TestCommandMayStopReadingInput(t *testing.T) {
	r, _, _, err := runLocal(t, []string{"true"}, bytes.NewReader(make([]byte, 2<<20)), false)
	if err != nil || r.ExitCode() != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

// Delaying the downstream reader must neither truncate buffered output nor
// terminate the transport after the command has already exited.
func TestSlowOutputDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	a, b := net.Pipe()
	go Serve(ctx, b, "instance")
	output, writer := io.Pipe()
	defer output.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Execute(ctx, a, Request{Version: ProtocolVersion, InstanceID: "instance", Argv: []string{"head", "-c", "1048576", "/dev/zero"}}, Options{IO: IO{Stdout: writer}})
		writer.CloseWithError(err)
		done <- err
	}()
	time.Sleep(4 * time.Second)
	n, err := io.Copy(io.Discard, output)
	if err != nil || n != 1048576 {
		t.Fatalf("read %d: %v", n, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAckIgnoresLateResize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan struct{})
	go func() { Serve(ctx, b, "instance"); close(done) }()
	s, err := Open(ctx, a, Request{Version: ProtocolVersion, InstanceID: "instance", Argv: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Session.Close()
	for {
		var m Message
		if err := s.Decoder.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m.Type == "completed" {
			break
		}
	}
	s.Send(Message{Type: "resize", Rows: 24, Cols: 80})
	select {
	case <-done:
		t.Fatal("accepted unrelated control message as ack")
	case <-time.After(100 * time.Millisecond):
	}
	s.Send(Message{Type: "ack"})
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("ack did not complete execution")
	}
}

func TestServerShutdownAfterCommandReady(t *testing.T) {
	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	clientCtx, stopClient := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopClient()
	a, b := net.Pipe()
	served := make(chan struct{})
	go func() { Serve(serverCtx, b, "instance"); close(served) }()
	output, writer := io.Pipe()
	defer output.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Execute(clientCtx, a, Request{Version: ProtocolVersion, InstanceID: "instance", Argv: []string{"sh", "-c", "trap '' TERM; printf ready; exec sleep 100"}}, Options{IO: IO{Stdout: writer}})
		writer.CloseWithError(err)
		done <- err
	}()
	ready := make([]byte, 5)
	if _, err := io.ReadFull(output, ready); err != nil {
		t.Fatal(err)
	}
	stopServer()
	select {
	case <-served:
	case <-time.After(6 * time.Second):
		t.Fatal("server retained execution")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled execution reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("client retained execution after server shutdown")
	}
}
