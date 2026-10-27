package service

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	guestvsock "guestAgent/pkg/vsock"
)

// StartGuestControlServer exposes the guest command runner over a dedicated
// vsock port. Its context owns every upgraded execution until cleanup finishes.
var guestInstance = uuid.NewString()

func StartGuestControlServer(ctx context.Context, container bool) error {
	var commandEnv []string
	if container {
		commandEnv = []string{"CONTAINER_HOST=unix://" + define.GuestPodmanSocket}
		// Commands use the already supervised engine, with no mount privileges.
		// Publish readiness only after that engine can answer requests.
		client := network.NewUnixClient(define.GuestPodmanSocket, network.WithTimeout(time.Second))
		defer client.Close()
		for {
			_, status, err := client.Get("/_ping").DoAndRead(ctx)
			if err == nil && status == 200 {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	listener, err := guestvsock.ListenHost(define.GuestControlPort)
	if err != nil {
		return fmt.Errorf("listen guest control vsock: %w", err)
	}

	return serveGuestControl(ctx, listener, commandEnv)
}

func serveGuestControl(ctx context.Context, listener net.Listener, commandEnv []string) error {
	ctx, cancel := context.WithCancel(ctx)
	var gate sync.Mutex
	var handlers sync.WaitGroup
	closing := false
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gate.Lock()
		if closing {
			gate.Unlock()
			http.Error(w, "guest stopping", http.StatusServiceUnavailable)
			return
		}
		handlers.Add(1)
		gate.Unlock()
		defer handlers.Done()
		handleGuestControl(ctx, w, r, commandEnv...)
	})}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		gate.Lock()
		closing = true
		gate.Unlock()
		shutdown, stop := context.WithTimeout(context.Background(), 4*time.Second)
		defer stop()
		_ = srv.Shutdown(shutdown)
		_ = srv.Close()
		handlers.Wait() // HTTP Shutdown does not track hijacked connections.
	}()
	logrus.Infof("guest control listening on vsock port %d", define.GuestControlPort)
	err := srv.Serve(listener)
	cancel()
	<-stopped
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("guest control server: %w", err)
	}
	return ctx.Err()
}

func handleGuestControl(ctx context.Context, w http.ResponseWriter, r *http.Request, commandEnv ...string) {
	if r.URL.Path == "/v2/ready" {
		_ = json.NewEncoder(w).Encode(protocol.ExecReady{Version: 2, InstanceID: guestInstance})
		return
	}
	if r.URL.Path == "/v2/exec" {
		if r.Header.Get("Upgrade") != "revm-exec-v2" {
			http.Error(w, "exec v2 upgrade required", 400)
			return
		}
		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "upgrade unavailable", 500)
			return
		}
		conn, rw, err := h.Hijack()
		if err != nil {
			return
		}
		_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: revm-exec-v2\r\n\r\n")
		if err != nil || rw.Flush() != nil {
			conn.Close()
			return
		}
		execution.Serve(ctx, conn, guestInstance, commandEnv...)
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}
