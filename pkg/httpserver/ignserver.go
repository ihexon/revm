//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package httpserver

import (
	"context"
	"net/http"

	"linuxvm/pkg/vmbuilder"
)

type GuestConfigServer struct {
	vmc *vmbuilder.VMConfig
	srv *httpServer
}

// NewIgnitionServer creates a httpserver that provides configuration to the guest.
func NewIgnitionServer(vmc *vmbuilder.VMConfig) *GuestConfigServer {
	return &GuestConfigServer{
		vmc: vmc,
		srv: newUnixSockHTTPServer("ignition-httpserver", vmc.IgnitionServerCfg.ListenSockAddr),
	}
}

func (s *GuestConfigServer) Start(ctx context.Context) error {
	s.srv.mux.HandleFunc("/healthz", s.handleHealth)
	s.srv.mux.HandleFunc("/vmconfig", s.handleVMConfig)

	return s.srv.serve(ctx)
}

func (s *GuestConfigServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, nil)
}

func (s *GuestConfigServer) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, s.vmc)
}
