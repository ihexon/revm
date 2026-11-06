package podmanproxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type observedListener struct {
	net.Listener
	accepted chan net.Conn
}

func (l observedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.accepted <- conn
	}
	return conn, err
}

func TestIndependentConnectionsHalfCloseAndShutdown(t *testing.T) {
	// macOS limits Unix socket paths to 104 bytes.
	dir, err := os.MkdirTemp("/tmp", "revm-proxy-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enginePath, transportPath, apiPath := filepath.Join(dir, "engine"), filepath.Join(dir, "transport"), filepath.Join(dir, "api")
	engine, err := net.Listen("unix", enginePath)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	transport, err := net.Listen("unix", transportPath)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	accepted := make(chan net.Conn, 16)
	go func() { done <- ServeGuest(ctx, observedListener{transport, accepted}, enginePath) }()
	go func() { done <- ServeHost(ctx, apiPath, transportPath) }()
	go func() {
		for {
			conn, err := engine.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				stop := context.AfterFunc(ctx, func() { conn.Close() })
				defer stop()
				data, err := io.ReadAll(conn)
				if err == nil {
					_, _ = conn.Write(data)
				}
			}()
		}
	}()
	dial := func() *net.UnixConn {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.Dial("unix", apiPath)
			if err == nil {
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				return conn.(*net.UnixConn)
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	nextTransport := func() net.Conn {
		t.Helper()
		select {
		case conn := <-accepted:
			return conn
		case <-time.After(5 * time.Second):
			t.Fatal("client did not receive its own transport")
			return nil
		}
	}
	// Larger than frames and socket buffers: EOF must not truncate output.
	// Concurrent clients have separate transports.
	payload := bytes.Repeat([]byte("full-duplex\n"), 400000)
	var calls sync.WaitGroup
	for range 4 {
		conn := dial()
		calls.Go(func() {
			defer conn.Close()
			if _, err := conn.Write(payload); err != nil {
				t.Error(err)
				return
			}
			if err := conn.CloseWrite(); err != nil {
				t.Error(err)
				return
			}
			got, err := io.ReadAll(conn)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("echo truncated: got %d bytes, want %d", len(got), len(payload))
			}
		})
	}
	calls.Wait()
	for range 4 {
		nextTransport()
	}
	// A failed transport belongs to one client and cannot interrupt another.
	lost := dial()
	defer lost.Close()
	lostTransport := nextTransport()
	survivor := dial()
	defer survivor.Close()
	nextTransport()
	lostTransport.Close()
	if _, err := lost.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("failed transport did not close its client: %v", err)
	}
	if _, err := survivor.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := survivor.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(survivor); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("another connection was affected: bytes=%d error=%v", len(got), err)
	}
	// An idle client must not prevent cancellation from draining the proxies.
	idle := dial()
	defer idle.Close()
	_, _ = idle.Write([]byte("unfinished"))
	cancel()
	for range 2 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("proxy shutdown blocked on an idle connection")
		}
	}
}
