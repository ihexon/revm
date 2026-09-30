//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

import (
	"context"
	"encoding/json"
	"fmt"
	httpv2 "linuxvm/pkg/http"
	"linuxvm/pkg/protocol"
	"net/http"
)

type Server struct {
	srv     *httpv2.Server
	machine Machine
}

type errResponse struct {
	Error string `json:"error"`
}

type Machine interface {
	RequestShutdown(ctx context.Context) error
	ManagementView() VMConfigView
	AttachSpec() protocol.AttachSpec
}

func writeJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value) //nolint:errchkjson
}

func NewServer(machine Machine) (*Server, error) {
	if machine == nil {
		return nil, fmt.Errorf("machine is nil")
	}
	config := machine.ManagementView()
	if config.Endpoints.ManagementAPI == "" {
		return nil, fmt.Errorf("management API endpoint is empty")
	}
	return &Server{
		machine: machine,
		srv:     httpv2.NewUnixSockHTTPServer("management-api", config.Endpoints.ManagementAPI),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.srv.Mux.HandleFunc("/v2/healthz", s.handleHealth)
	s.srv.Mux.HandleFunc("/v2/vmconfig", s.handleVMConfig)
	s.srv.Mux.HandleFunc("/v2/attach", s.handleAttach)
	s.srv.Mux.HandleFunc("/v2/stop", s.handleRequestVMStop)
	return s.srv.Serve(ctx)
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
	writeJSON(w, http.StatusOK, s.machine.ManagementView())
}

func (s *Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	writeJSON(w, http.StatusOK, s.machine.AttachSpec())
}

func (s *Server) handleRequestVMStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	if err := s.machine.RequestShutdown(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
