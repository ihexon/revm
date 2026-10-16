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

func TestMultiplexedHalfCloseAndShutdown(t *testing.T) {
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
	go func() { done <- ServeGuest(ctx, transport, enginePath) }()
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
	// Larger than Yamux's initial window: EOF must not disable return-path
	// credit updates or truncate output. Concurrent clients share one session.
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
