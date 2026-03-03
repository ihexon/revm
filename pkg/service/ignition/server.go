//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package ignition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	http2 "linuxvm/pkg/http"
	"linuxvm/pkg/network"
	"net/http"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/sirupsen/logrus"
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
	s.srv.Mux.HandleFunc("/ready/{service}", s.handleReady)

	errChan := make(chan error, 2)
	go func() {
		if err := s.waitVirtualNetworkOnline(ctx); err != nil {
			errChan <- err
		}
	}()
	go func() {
		<-s.vmc.Readiness.SSHReady
		<-s.vmc.Readiness.PodmanReady
		<-s.vmc.Readiness.VNetHostReady
		<-s.vmc.Readiness.VNetGuestReady
	}()
	go func() { errChan <- s.srv.Serve(ctx) }()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) waitTSINetworkOnline(ctx context.Context) error {
	if s.vmc.Readiness.SignalVNetHostReady() {
		logrus.Infof("[ign] TSI network online")
	}
	return nil
}

func (s *Server) waitGvisorVSockTapOnline(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(s.vmc.GVPCtlAddr)
	if err != nil {
		return err
	}
	client := network.NewUnixClient(addr.Path, network.WithTimeout(define.DefaultTimeTicker))
	defer client.Close()
	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get("/services/forwarder/all").Do(ctx) //nolint:bodyclose
			if err != nil {
				continue
			}
			network.CloseResponse(resp)
			if resp.StatusCode == http.StatusOK {
				if s.vmc.Readiness.SignalVNetHostReady() {
					logrus.Infof("[ign] gvisor virtual-network online")
				}
				return nil
			}
		}
	}
}

func (s *Server) waitVirtualNetworkOnline(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, define.DefaultProbeTimeout)
	defer cancel()
	switch s.vmc.VirtualNetworkMode {
	case define.TSI:
		return s.waitTSINetworkOnline(ctx)
	case define.GVISOR:
		return s.waitGvisorVSockTapOnline(ctx)
	default:
		return fmt.Errorf("unknown virtual network mode: %s", s.vmc.VirtualNetworkMode)
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

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	switch r.PathValue("service") {
	case define.ServiceNameSSH:
		if s.vmc.Readiness.SignalSSHReady() {
			logrus.Info("[ign] guest ssh server online")
		}
	case define.ServiceNamePodman:
		if s.vmc.Readiness.SignalPodmanAPIProxyReady() {
			logrus.Info("[ign] guest podman online")
		}
	case define.ServiceNameGuestNetwork:
		if s.vmc.Readiness.SignalVNetGuestReady() {
			logrus.Info("[ign] guest network online")
		}
	default:
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}
