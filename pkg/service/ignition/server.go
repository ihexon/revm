//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package ignition

import (
	"context"
	"encoding/json"
	"fmt"
	http2 "linuxvm/pkg/http"
	"linuxvm/pkg/protocol"
	"net/http"
)

type Server struct {
	machine Machine
	srv     *http2.Server

	Listening chan struct{}
}

type Machine interface {
	IgnitionListenAddr() string
	GuestSpec() protocol.GuestSpec
}

func NewServer(machine Machine) (*Server, error) {
	if machine == nil {
		return nil, fmt.Errorf("machine is nil")
	}
	if machine.IgnitionListenAddr() == "" {
		return nil, fmt.Errorf("ignition endpoint is empty")
	}

	s := &Server{machine: machine, Listening: make(chan struct{})}
	srv := http2.NewUnixSockHTTPServer("ignition-httpserver", machine.IgnitionListenAddr())
	srv.OnListening = func() { close(s.Listening) }
	s.srv = srv
	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.srv.Mux.HandleFunc("/healthz", s.handleHealth)
	s.srv.Mux.HandleFunc("/vmconfig", s.handleVMConfig)

	errChan := make(chan error, 2)
	go func() { errChan <- s.srv.Serve(ctx) }()

	select {
	case err := <-errChan:
		return fmt.Errorf("ignition server: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value) //nolint:errchkjson
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (s *Server) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	writeJSON(w, http.StatusOK, s.machine.GuestSpec())
}
