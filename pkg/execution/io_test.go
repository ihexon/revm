package execution

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestExecutionPreservesBlockingOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	go Serve(ctx, b, "instance")
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	reader, output, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer output.Close()
	fd := output.Fd()
	before, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Execute(ctx, a, Request{Version: 2, InstanceID: "instance", RequestID: "flags", Argv: []string{"cat"}}, Options{IO: IO{Stdin: input, Stdout: output}})
		done <- err
	}()
	go func() { _, _ = writer.Write([]byte("ready")) }()
	_ = reader.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(reader, make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	after, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err != nil || after&unix.O_NONBLOCK != before&unix.O_NONBLOCK {
		t.Fatalf("execution changed shared output flags: before=%d after=%d err=%v", before, after, err)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
