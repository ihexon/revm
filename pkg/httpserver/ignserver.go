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

	Listening chan struct{}

	SSHReady chan struct{}
	sshOnce  sync.Once

	PodmanReady chan struct{}
	podmanOnce  sync.Once

	// VNetHostReady indicates the host-side virtual network(TSI, gvisor-tap-vsock)
	VNetHostReady chan struct{}
	vNetHostOnce  sync.Once

	// VNetHostReady indicates the guest-side virtual network(bring up guest interface and get IP)
	VNetGuestReady chan struct{}
	vNetGuestOnce  sync.Once
}

// NewIgnServer creates a httpserver that provides configuration to the guest.
func NewIgnServer(vmc *vmbuilder.VMConfig) *IgnServer {
	s := &IgnServer{
		vmc:            vmc,
		Listening:      make(chan struct{}),
		SSHReady:       make(chan struct{}),
		PodmanReady:    make(chan struct{}),
		VNetHostReady:  make(chan struct{}),
		VNetGuestReady: make(chan struct{}),
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
	s.srv.mux.HandleFunc(fmt.Sprintf("/ready/%s", define.ServiceNameGuestNetwork), s.handleReadyGuestNetwork)

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

func (s *IgnServer) waitTSINetworkOnline(ctx context.Context) error {
	s.vNetHostOnce.Do(func() {
		logrus.Infof("[ign] TSI network online")
		close(s.VNetHostReady)
	})
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
				s.vNetHostOnce.Do(func() {
					logrus.Infof("[ign] gvisor virtual-network online")
					close(s.VNetHostReady)
				})
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
	s.sshOnce.Do(func() {
		logrus.Info("[ign] guest ssh server online")
		close(s.SSHReady)
	})
	WriteJSON(w, http.StatusOK, nil)
}

func (s *IgnServer) handleReadyPodman(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	s.podmanOnce.Do(func() {
		logrus.Infof("[ign] guest podman online")
		close(s.PodmanReady)
	})
	WriteJSON(w, http.StatusOK, nil)
}

func (s *IgnServer) handleReadyGuestNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	s.vNetGuestOnce.Do(func() {
		logrus.Infof("[ign] guest network online")
		close(s.VNetGuestReady)
	})
	WriteJSON(w, http.StatusOK, nil)
}
