//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	http2 "linuxvm/pkg/http"
	"net/http"
	"sync"
	"time"

	"linuxvm/pkg/network"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/sirupsen/logrus"
)

type IgnServer struct {
	mu  sync.RWMutex
	vmc *define.Machine
	srv *http2.Server

	Listening chan struct{}
}

// NewIgnServer creates a httpserver that provides configuration to the guest.
func NewIgnServer(vmc *define.Machine) *IgnServer {
	s := &IgnServer{
		vmc:       vmc,
		Listening: make(chan struct{}),
	}

	srv := http2.NewUnixSockHTTPServer("ignition-httpserver", vmc.IgnitionServerCfg.ListenSockAddr)
	srv.OnListening = func() { close(s.Listening) }
	s.srv = srv
	return s
}

func (s *IgnServer) Start(ctx context.Context) error {
	event.Emit(event.StartIgnitionServer)

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
		event.Emit(event.AllThingsReady)
	}()

	go func() {
		errChan <- s.srv.Serve(ctx)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *IgnServer) waitTSINetworkOnline(ctx context.Context) error {
	if s.vmc.Readiness.SignalVNetHostReady() {
		logrus.Infof("[ign] TSI network online")
		event.Emit(event.HostNetworkReady)
	}
	return nil
}

func (s *IgnServer) waitGvisorVSockTapOnline(ctx context.Context) error {
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
					event.Emit(event.HostNetworkReady)
				}
				return nil
			}
		}
	}
}

// waitVirtualNetworkOnline must support TSI/Gvisor network mode
func (s *IgnServer) waitVirtualNetworkOnline(ctx context.Context) error {
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

func (s *IgnServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, nil)
}

func (s *IgnServer) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		WriteJSON(w, http.StatusOK, s.vmc)
	case http.MethodPatch:
		s.mu.Lock()
		defer s.mu.Unlock()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, err.Error())
			return
		}

		currentBytes, err := json.Marshal(s.vmc)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		mergedBytes, err := jsonpatch.MergePatch(currentBytes, body)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		if err = json.Unmarshal(mergedBytes, s.vmc); err != nil {
			WriteJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, nil)
	default:
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *IgnServer) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	switch r.PathValue("service") {
	case define.ServiceNameSSH:
		if s.vmc.Readiness.SignalSSHReady() {
			logrus.Info("[ign] guest ssh server online")
			event.Emit(event.GuestSSHReady)
		}
	case define.ServiceNamePodman:
		if s.vmc.Readiness.SignalPodmanAPIProxyReady() {
			logrus.Info("[ign] guest podman online")
			event.Emit(event.GuestPodmanReady)
		}
	case define.ServiceNameGuestNetwork:
		if s.vmc.Readiness.SignalVNetGuestReady() {
			logrus.Info("[ign] guest network online")
			event.Emit(event.GuestNetworkReady)
		}
	default:
		WriteJSON(w, http.StatusNotFound, nil)
		return
	}
	WriteJSON(w, http.StatusOK, nil)
}
