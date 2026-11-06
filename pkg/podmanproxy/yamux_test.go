package podmanproxy

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
)

func TestCanceledOpenPreservesSession(t *testing.T) {
	a, b := net.Pipe()
	cfg := muxConfig()
	cfg.AcceptBacklog = 1
	client, err := yamux.Client(a, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := yamux.Server(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	// Leave the first stream unacknowledged, filling the SYN backlog.
	first, err := client.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	dialCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := openStream(dialCtx, client); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked dial: %v", err)
	}
	if client.IsClosed() {
		t.Fatal("canceling a dial closed the shared session")
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	accepted, err := server.AcceptStreamWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	// Once the backlog clears, the abandoned open must send EOF to its peer.
	abandoned, err := server.AcceptStreamWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer abandoned.Close()
	_ = abandoned.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := abandoned.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("abandoned stream was not closed: %v", err)
	}
	// Close is a half-close: the peer must finish its half too, just as the
	// proxy's connection handler does. If FIN arrives before Accept sends ACK,
	// the pending SYN keeps the sole backlog slot until both halves close.
	// Deferring this close until after the next dial makes the test deadlock.
	if err := abandoned.Close(); err != nil {
		t.Fatal(err)
	}
	// Other streams still work, and a full local Close unblocks their reads.
	conn, err := openStream(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	peer, err := server.AcceptStreamWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if _, err := peer.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 2)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "ok" {
		t.Fatalf("shared session lost data: %q %v", data, err)
	}
	done := make(chan error, 1)
	go func() { _, err := conn.Read(data); done <- err }()
	_ = conn.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read succeeded after Close")
		}
	case <-ctx.Done():
		t.Fatal("Close did not unblock read")
	}
}
