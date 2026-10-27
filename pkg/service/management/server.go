//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/network"
	"net"
	"net/http"
	"os"
)

type Server struct {
	socket  string
	machine Machine
}

type errResponse struct {
	Error string `json:"error"`
}

type Machine interface {
	RequestStop(ctx context.Context) error
	ManagementView() VMConfigView
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
	addr, err := network.ParseUnixAddr(machine.ManagementView().Endpoints.ManagementAPI)
	if err != nil {
		return nil, err
	}
	return &Server{machine: machine, socket: addr.Path}, nil
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(s.socket, 0600); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/healthz", s.handleHealth)
	mux.HandleFunc("/v2/vmconfig", s.handleVMConfig)
	mux.HandleFunc("/v2/stop", s.handleRequestVMStop)
	server := &http.Server{Handler: mux}
	defer server.Close()
	stop := context.AfterFunc(ctx, func() { server.Close() })
	defer stop()
	return server.Serve(listener)
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

func (s *Server) handleRequestVMStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	if err := s.machine.RequestStop(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
