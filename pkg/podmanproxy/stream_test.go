package podmanproxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestFramedHalfCloseAndDrain(t *testing.T) {
	a, b := net.Pipe()
	client, server := &framedConn{Conn: a}, &framedConn{Conn: b}
	defer client.Close()
	defer server.Close()
	deadline := time.Now().Add(5 * time.Second)
	client.SetDeadline(deadline)
	server.SetDeadline(deadline)
	payload := bytes.Repeat([]byte{0, 1, 255}, maxFrameSize*10)
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		request, err := io.ReadAll(server)
		if err == nil && string(request) != "request" {
			err = errors.New("request was truncated")
		}
		if err == nil {
			_, err = server.Write(payload)
		}
		if err == nil {
			err = server.CloseWrite()
		}
		done <- err
	}()
	if _, err := client.Write(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write after EOF: %v", err)
	}
	got, err := io.ReadAll(client)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("response truncated: bytes=%d error=%v", len(got), err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFramedInputValidation(t *testing.T) {
	frame := func(size uint32, data string) []byte {
		return append(binary.BigEndian.AppendUint32(nil, size), data...)
	}
	for _, tc := range []struct {
		name string
		wire []byte
		want error
	}{
		{"EOF", frame(0, ""), nil},
		{"missing EOF", frame(1, "x"), io.ErrUnexpectedEOF},
		{"partial header", []byte{0}, io.ErrUnexpectedEOF},
		{"partial data", frame(2, "x"), io.ErrUnexpectedEOF},
		{"missing data", frame(1, ""), io.ErrUnexpectedEOF},
		{"oversized", frame(maxFrameSize+1, ""), errors.New("oversized")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(time.Second))
			go func() { b.Write(tc.wire); b.Close() }()
			_, err := io.ReadAll(&framedConn{Conn: a})
			if tc.name == "oversized" {
				if err == nil {
					t.Fatal("oversized frame accepted")
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("read: %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCancellationDuringDialClosesClient(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}), make(chan struct{})
	go func() {
		proxyConnection(ctx, &framedConn{Conn: a}, func(ctx context.Context) (halfCloseConn, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceling the dial blocked")
	}
	b.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := b.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("canceled dial retained its client: %v", err)
	}
}
