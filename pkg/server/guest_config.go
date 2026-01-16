//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"context"
	"net/http"

	"linuxvm/pkg/vmconfig"

	"github.com/sirupsen/logrus"
)

// GuestConfigServer provides VM configuration to the guest agent.
// It listens on a VSock port, allowing the guest to fetch its configuration
// during the provisioning phase.
//
// Endpoints:
//   - GET /healthz  - Health check
//   - GET /vmconfig - Returns the complete VM configuration as JSON
type GuestConfigServer struct {
	vmc *vmconfig.VMConfig
	srv *httpServer
}

// NewGuestConfigServer creates a server that provides configuration to the guest.
func NewGuestConfigServer(vmc *vmconfig.VMConfig) *GuestConfigServer {
	return &GuestConfigServer{
		vmc: vmc,
		srv: newHTTPServer("guest-config", vmc.IgnProvisionerAddr),
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
	logrus.Infof("guest-config: /healthz")
	WriteJSON(w, http.StatusOK, nil)
}

func (s *GuestConfigServer) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	logrus.Infof("guest-config: /vmconfig")
	WriteJSON(w, http.StatusOK, s.vmc)
}
