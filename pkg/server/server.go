//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig"
	"net"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

type Server struct {
	Vmc      *vmconfig.VMConfig
	Server   *http.Server
	Mux      *http.ServeMux
	UnixAddr string
}

func NewAPIServer(vmc *vmconfig.VMConfig) *Server {
	mux := http.NewServeMux()
	server := &Server{
		Mux:      mux,
		Vmc:      vmc,
		UnixAddr: vmc.RestAPIAddress,
	}

	return server
}

func (s *Server) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("handle /vmconfig request")
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	WriteJSON(w, http.StatusOK, s.Vmc)
}

const (
	guestexecURL = "/exec"
	vmconfigURL  = "/vmconfig"
)

func (s *Server) registerRouter() {
	s.Mux.HandleFunc(vmconfigURL, s.handleVMConfig)
	s.Mux.HandleFunc(guestexecURL, s.doExec)
}

func (s *Server) Start(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(s.UnixAddr)
	if err != nil {
		return fmt.Errorf("failed to parse unix socket address: %w", err)
	}

	if err = os.Remove(addr.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old unix socket %q: %w", s.UnixAddr, err)
	}

	ln, err := net.Listen("unix", addr.Path)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %q: %w", s.UnixAddr, err)
	}

	defer func() {
		_ = os.Remove(addr.Path)
	}()

	s.registerRouter()

	s.Server = &http.Server{
		Handler: s.Mux,
	}

	errChan := make(chan error, 1)

	go func() {
		logrus.Infof("start revm API server on %q", ln.Addr().String())
		if err = s.Server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	select {
	case err = <-errChan:
		return fmt.Errorf("start rest server error: %w", err)
	case <-ctx.Done():
		logrus.Infof("close rest server on %q", ln.Addr().String())
		_ = s.Server.Close()
		_ = ln.Close()
		return context.Cause(ctx)
	}
}

// WriteJSON writes an interface value encoded as JSON to w
func WriteJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	coder := json.NewEncoder(w)
	coder.SetEscapeHTML(true)
	if err := coder.Encode(value); err != nil {
		logrus.Errorf("unable to encode json: %q", err)
	}
}
