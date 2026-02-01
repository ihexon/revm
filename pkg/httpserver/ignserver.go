//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package httpserver

import (
	"context"
	"net/http"

	"linuxvm/pkg/vmconfig"
)

type GuestConfigServer struct {
	vmc *vmconfig.VMConfig
	srv *httpServer
	elfFS   http.Handler
}

// NewIgnitionServer creates a httpserver that provides configuration to the guest.
func NewIgnitionServer(vmc *vmconfig.VMConfig) *GuestConfigServer {
	return &GuestConfigServer{
		vmc: vmc,
		srv: newUnixSockHTTPServer("ignition-httpserver", vmc.Ignition.HostListenAddr),
	}
}

// Start begins serving requests. Blocks until context is cancelled.
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