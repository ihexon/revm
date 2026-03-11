//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"linuxvm/pkg/network"

	"github.com/sirupsen/logrus"
)

// Server provides common HTTP server functionality.
type Server struct {
	name        string
	listener    string
	server      *http.Server
	Mux         *http.ServeMux
	OnListening func() // called once after net.Listen succeeds
}

func NewUnixSockHTTPServer(name, listener string) *Server {
	return &Server{
		name:     name,
		listener: listener,
		Mux:      http.NewServeMux(),
	}
}

// Serve starts the HTTP server and blocks until context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(s.listener)
	if err != nil {
		return fmt.Errorf("failed to parse unix socket address: %w", err)
	}

	_ = os.Remove(addr.Path)

	ln, err := net.Listen("unix", addr.Path)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", addr.Path, err)
	}
	defer os.Remove(addr.Path)

	if s.OnListening != nil {
		s.OnListening()
	}

	s.server = &http.Server{Handler: s.Mux}

	errChan := make(chan error, 1)
	go func() {
		logrus.Infof("starting %s httpserver on %q", s.name, ln.Addr().String())
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		_ = ln.Close()
		logrus.Infof("%s httpserver stopped", s.name)
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("%s httpserver error: %w", s.name, err)
	case <-ctx.Done():
		return ctx.Err()
	}
}
