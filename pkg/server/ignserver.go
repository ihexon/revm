package server

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
)

type Server struct {
	Vmc        *vmconfig.VMConfig
	Server     *http.Server
	Mux        *http.ServeMux
	ListenAddr url.URL
	Ctx        context.Context
}

func NewServer(ctx context.Context, vmc *vmconfig.VMConfig) *Server {
	mux := http.NewServeMux()
	server := &Server{
		Mux:        mux,
		Vmc:        vmc,
		ListenAddr: url.URL{Scheme: "http", Host: define.DefaultRestAddr},
		Ctx:        ctx,
	}
	return server
}

func (s *Server) handleShowMounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, s.Vmc.Mounts)
}

func (s *Server) registerRouter() {
	s.Mux.HandleFunc("/host/mounts", s.handleShowMounts)
}

func (s *Server) Start() error {
	s.registerRouter()

	s.Server = &http.Server{
		Addr:         s.ListenAddr.Host,
		Handler:      s.Mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	errChan := make(chan error, 1)

	go func() {
		logrus.Infof("start server on %q", s.ListenAddr.String())
		if err := s.Server.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("start rest server error: %w", err)
	case <-s.Ctx.Done():
		logrus.Infof("close rest server on %q", s.ListenAddr.String())
		return s.Server.Close()
	}
}

// WriteJSON writes an interface value encoded as JSON to w
func WriteJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	coder := json.NewEncoder(w)
	coder.SetEscapeHTML(true)
	if err := coder.Encode(value); err != nil {
		logrus.Errorf("Unable to write json: %q", err)
	}
}
