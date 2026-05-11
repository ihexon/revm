//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package ignition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	http2 "linuxvm/pkg/http"
	"net/http"
	"sync"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

type Server struct {
	mu  sync.RWMutex
	vmc *define.Machine
	srv *http2.Server

	Listening chan struct{}
}

func NewServer(vmc *define.Machine) *Server {
	s := &Server{vmc: vmc, Listening: make(chan struct{})}
	srv := http2.NewUnixSockHTTPServer("ignition-httpserver", vmc.IgnitionServerCfg.ListenSockAddr)
	srv.OnListening = func() { close(s.Listening) }
	s.srv = srv
	return s
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
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, http.StatusOK, s.vmc)
	case http.MethodPatch:
		s.mu.Lock()
		defer s.mu.Unlock()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		currentBytes, err := json.Marshal(s.vmc)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		mergedBytes, err := jsonpatch.MergePatch(currentBytes, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		if err = json.Unmarshal(mergedBytes, s.vmc); err != nil {
			writeJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, nil)
	}
}
