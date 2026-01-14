//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"linuxvm/pkg/network"

	"github.com/sirupsen/logrus"
)

// httpServer provides common HTTP server functionality.
type httpServer struct {
	name     string
	listener string
	server   *http.Server
	mux      *http.ServeMux
}

func newHTTPServer(name, listener string) *httpServer {
	return &httpServer{
		name:     name,
		listener: listener,
		mux:      http.NewServeMux(),
	}
}

// serve starts the HTTP server and blocks until context is cancelled.
func (s *httpServer) serve(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(s.listener)
	if err != nil {
		return fmt.Errorf("failed to parse unix socket address: %w", err)
	}

	if err = os.Remove(addr.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old unix socket %q: %w", addr.Path, err)
	}

	ln, err := net.Listen("unix", addr.Path)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %w", addr.Path, err)
	}
	defer os.Remove(addr.Path)

	s.server = &http.Server{Handler: s.mux}

	errChan := make(chan error, 1)
	go func() {
		logrus.Infof("starting %s server on %q", s.name, ln.Addr().String())
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	defer func() {
		_ = s.server.Close()
		_ = ln.Close()
		logrus.Debugf("%s server stopped", s.name)
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("%s server error: %w", s.name, err)
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
