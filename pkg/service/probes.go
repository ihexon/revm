package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	defaultProbeTimeout  = 50 * time.Millisecond
	defaultProbeInterval = 50 * time.Millisecond

	// podman/ssh takes much longer to start than the other services
	defaultPodmanProbeTimeout = 1 * time.Second
	defaultSSHProbeTimeout    = 1 * time.Second
)

type Probe interface {
	ProbeUntilReady(ctx context.Context) error
}

type GVProxyProbe struct {
	unixURL string
	Ch      chan struct{}
	once    sync.Once
}

func NewGVProxyProbe(unixURL string) *GVProxyProbe {
	return &GVProxyProbe{
		unixURL: unixURL,
		Ch:      make(chan struct{}, 1),
	}
}

func (g *GVProxyProbe) ProbeUntilReady(ctx context.Context) error {
	// Fast-path: already ready
	select {
	case <-g.Ch:
		return nil
	default:
	}

	socketPath, err := network.ParseUnixAddr(g.unixURL)
	if err != nil {
		return fmt.Errorf("invalid unix URL %q: %w", g.unixURL, err)
	}

	client := network.NewUnixClient(socketPath.Path, network.WithTimeout(defaultProbeTimeout))
	defer client.Close()

	ticker := time.NewTicker(defaultProbeInterval)
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
				g.once.Do(func() { close(g.Ch) })
				logrus.Info("gvproxy service is ready")
				return nil
			}
		}
	}
}

type IgnServerProbe struct {
	unixURL string
	Ch      chan struct{}
	once    sync.Once
}

func NewIgnServerProbe(unixURL string) *IgnServerProbe {
	return &IgnServerProbe{
		unixURL: unixURL,
		Ch:      make(chan struct{}, 1),
	}
}

func (p *IgnServerProbe) ProbeUntilReady(ctx context.Context) error {
	// Fast-path: already ready
	select {
	case <-p.Ch:
		return nil
	default:
	}

	socketPath, err := network.ParseUnixAddr(p.unixURL)
	if err != nil {
		return fmt.Errorf("invalid unix URL %q: %w", p.unixURL, err)
	}

	client := network.NewUnixClient(socketPath.Path, network.WithTimeout(defaultProbeTimeout))
	defer client.Close()

	ticker := time.NewTicker(defaultProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get("/healthz").Do(ctx) //nolint:bodyclose
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				logrus.Warnf("ignition server /healthz returned status code: %d, retrying", resp.StatusCode)
				network.CloseResponse(resp)
				continue
			}

			network.CloseResponse(resp)
			p.once.Do(func() { close(p.Ch) })
			logrus.Info("ignition server is ready")
			return nil
		}
	}
}

type GuestSSHProbe struct {
	vmc  *vmconfig.VMConfig
	Ch   chan struct{}
	once sync.Once
}

func NewGuestSSHProbe(vmc *vmconfig.VMConfig) *GuestSSHProbe {
	return &GuestSSHProbe{
		vmc: vmc,
		Ch:  make(chan struct{}, 1),
	}
}

func (p *GuestSSHProbe) ProbeUntilReady(ctx context.Context) error {
	// Fast-path: already ready
	select {
	case <-p.Ch:
		return nil
	default:
	}

	ticker := time.NewTicker(defaultSSHProbeTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			client, err := MakeSSHClient(ctx, p.vmc)
			if err != nil {
				logrus.Debugf("SSH probe failed: %v", err)
				continue
			}

			// run busybox to verify SSH is ready, remember to close the client
			if err = client.Run(ctx, filepath.Join(define.GuestHiddenBinDir, "busybox")); err != nil {
				_ = client.Close()
				logrus.Debugf("run busybox command failed: %v", err)
				continue
			}
			_ = client.Close()

			p.once.Do(func() {
				close(p.Ch)
			})
			logrus.Info("guest SSH service is ready")
			return nil
		}
	}
}

type PodmanProbe struct {
	vmc  *vmconfig.VMConfig
	Ch   chan struct{}
	once sync.Once
}

func NewPodmanProbe(vmc *vmconfig.VMConfig) *PodmanProbe {
	return &PodmanProbe{
		vmc: vmc,
		Ch:  make(chan struct{}, 1),
	}
}

func (p *PodmanProbe) ProbeUntilReady(ctx context.Context) error {
	// Fast-path: already ready
	select {
	case <-p.Ch:
		return nil
	default:
	}

	var (
		client *network.Client
	)

	if p.vmc.TSI {
		client = network.NewTCPClient(fmt.Sprintf("%s:%d", define.LocalHost, define.GuestPodmanAPIPort), network.WithTimeout(defaultProbeTimeout))
		defer client.Close()
	} else {
		socketPath, err := network.ParseUnixAddr(p.vmc.PodmanInfo.LocalPodmanProxyAddr)
		if err != nil {
			return err
		}
		client = network.NewUnixClient(socketPath.Path, network.WithTimeout(defaultProbeTimeout))
		defer client.Close()
	}

	ticker := time.NewTicker(defaultPodmanProbeTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get("/libpod/_ping").Do(ctx) //nolint:bodyclose
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				network.CloseResponse(resp)
				continue
			}

			network.CloseResponse(resp)
			p.once.Do(func() { close(p.Ch) })
			logrus.Info("Podman API service is ready")
			return nil
		}
	}
}

func WaitAll(ctx context.Context, probes ...Probe) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	for _, probe := range probes {
		p := probe
		g.Go(func() error {
			return p.ProbeUntilReady(ctx)
		})
	}

	return g.Wait()
}
