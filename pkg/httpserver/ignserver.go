//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package httpserver

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"net/http"
	"sync"
	"time"

	"linuxvm/pkg/network"
	"linuxvm/pkg/vmbuilder"

	"github.com/sirupsen/logrus"
)

type IgnServer struct {
	vmc *vmbuilder.VMConfig
	srv *httpServer

	Listening   chan struct{}
	SSHReady    chan struct{}
	PodmanReady chan struct{}
	VNetReady   chan struct{}

	sshOnce    sync.Once
	podmanOnce sync.Once
	vNetOnce   sync.Once
}

// NewIgnServer creates a httpserver that provides configuration to the guest.
func NewIgnServer(vmc *vmbuilder.VMConfig) *IgnServer {
	s := &IgnServer{
		vmc:         vmc,
		Listening:   make(chan struct{}),
		SSHReady:    make(chan struct{}),
		PodmanReady: make(chan struct{}),
		VNetReady:   make(chan struct{}),
	}
	srv := newUnixSockHTTPServer("ignition-httpserver", vmc.IgnitionServerCfg.ListenSockAddr)
	srv.onListening = func() { close(s.Listening) }
	s.srv = srv
	return s
}

func (s *IgnServer) Start(ctx context.Context) error {
	s.srv.mux.HandleFunc("/healthz", s.handleHealth)
	s.srv.mux.HandleFunc("/vmconfig", s.handleVMConfig)
	s.srv.mux.HandleFunc(fmt.Sprintf("/ready/%s", define.ServiceNameSSH), s.handleReadySSH)
	s.srv.mux.HandleFunc(fmt.Sprintf("/ready/%s", define.ServiceNamePodman), s.handleReadyPodman)
	errChan := make(chan error, 2)
	go func() {
		if err := s.waitVirtualNetworkOnline(ctx); err != nil {
			errChan <- err
		}
	}()
	go func() {
		errChan <- s.srv.serve(ctx)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitVirtualNetworkOnline must support TSI/Gvisor network mode
func (s *IgnServer) waitVirtualNetworkOnline(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	switch s.vmc.VirtualNetworkMode {
	case define.TSI.String():
		s.vNetOnce.Do(func() { close(s.VNetReady) })
		return nil
	case define.GVISOR.String():
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
					logrus.Infof("gvisor virtual-network online")
					s.vNetOnce.Do(func() { close(s.VNetReady) })
					return nil
				}
			}
		}
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
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, s.vmc)
}

func (s *IgnServer) handleReadySSH(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	s.sshOnce.Do(func() { close(s.SSHReady) })
	WriteJSON(w, http.StatusOK, nil)
}

func (s *IgnServer) handleReadyPodman(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	s.podmanOnce.Do(func() { close(s.PodmanReady) })
	WriteJSON(w, http.StatusOK, nil)
}
