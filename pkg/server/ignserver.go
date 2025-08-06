//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig"

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

type GVProxyInfo struct {
	ControlEndpoints    string `json:"gvproxy_control_endpoint,omitempty"`
	VFKitSocketEndpoint string `json:"gvproxy_network_endpoint,omitempty"`
}

func (s *Server) handleGVProxyInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	info := GVProxyInfo{
		ControlEndpoints:    s.Vmc.GVproxyEndpoint,
		VFKitSocketEndpoint: s.Vmc.NetworkStackBackend,
	}

	WriteJSON(w, http.StatusOK, info)
}
func (s *Server) handlePortForwardInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	info := s.Vmc.PortForwardMap
	WriteJSON(w, http.StatusOK, info)
}

type GuestInfo struct {
	MemoryInMb int32  `json:"memory,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootfsPath string `json:"rootfsPath,omitempty"`
}

func (s *Server) handleGuestInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	info := &GuestInfo{
		RootfsPath: s.Vmc.RootFS,
		Cpus:       s.Vmc.Cpus,
		MemoryInMb: s.Vmc.MemoryInMB,
	}

	WriteJSON(w, http.StatusOK, info)
}

func (s *Server) registerRouter() {
	s.Mux.HandleFunc("/host/mounts", s.handleShowMounts)
	s.Mux.HandleFunc("/guest/info", s.handleGuestInfo)
	s.Mux.HandleFunc("/network/info/gvproxy", s.handleGVProxyInfo)
	s.Mux.HandleFunc("/network/info/portmap", s.handlePortForwardInfo)
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
