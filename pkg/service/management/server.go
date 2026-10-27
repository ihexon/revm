//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

import (
	"context"
	"net"
	"net/http"
	"os"
)

func Serve(ctx context.Context, socket string, stopVM func()) error {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /v2/stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		stopVM()
	})
	server := &http.Server{Handler: mux}
	defer server.Close()
	stop := context.AfterFunc(ctx, func() { server.Close() })
	defer stop()
	return server.Serve(listener)
}
