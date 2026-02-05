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

// Probe defines the interface for service readiness probes.
type Probe interface {
	// ProbeUntilReady blocks until the service is ready or the context is cancelled.
	// Returns nil on success, ctx.Err() on context cancellation/timeout.
	ProbeUntilReady(ctx context.Context) error
}

// GVProxyProbe polls the gvproxy control socket until the service is ready.
// It uses HTTP GET /services/forwarder/all to verify gvproxy has started.
type GVProxyProbe struct {
	unixURL string
	Ch      chan struct{}
	once    sync.Once
}

// NewGVProxyProbe creates a new GVProxyProbe that monitors the given control socket.
// The unixURL can be either a unix:// URL or a raw socket path.
func NewGVProxyProbe(unixURL string) *GVProxyProbe {
	return &GVProxyProbe{
		unixURL: unixURL,
		Ch:      make(chan struct{}, 1),
	}
}

// ProbeUntilReady polls the gvproxy /services/forwarder/all endpoint until it returns HTTP 200.
// It blocks until the service is ready or the context is cancelled.
// The Ch channel is closed when the service becomes ready.
// Returns nil on success, ctx.Err() on context cancellation/timeout.
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

// IgnServerProbe polls the ignition server until it responds to health checks.
type IgnServerProbe struct {
	unixURL string
	Ch      chan struct{}
	once    sync.Once
}

// NewIgnServerProbe creates a new IgnServerProbe that monitors the given unix socket.
// The unixURL can be either a unix:// URL or a raw socket path.
func NewIgnServerProbe(unixURL string) *IgnServerProbe {
	return &IgnServerProbe{
		unixURL: unixURL,
		Ch:      make(chan struct{}, 1),
	}
}

// ProbeUntilReady polls the ignition server /healthz endpoint until it returns HTTP 200.
// It blocks until the service is ready or the context is cancelled.
// The Ch channel is closed when the service becomes ready.
// Returns nil on success, ctx.Err() on context cancellation/timeout.
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

// GuestSSHProbe polls the guest SSH service until it accepts connections.
// It connects through gvproxy's vsock tunnel to verify SSH is ready.
type GuestSSHProbe struct {
	// gvpCtlAddr is required to establish vsock tunnel to the guest SSH service.
	// See: https://github.com/containers/gvisor-tap-vsock/blob/main/cmd/ssh-over-vsock/main.go
	vmc  *vmconfig.VMConfig
	Ch   chan struct{}
	once sync.Once
}

// NewGuestSSHProbe creates a new GuestSSHProbe that monitors SSH readiness through the given gvproxy socket.
// The gvpCtlAddr can be either a unix:// URL or a raw socket path.
func NewGuestSSHProbe(vmc *vmconfig.VMConfig) *GuestSSHProbe {
	return &GuestSSHProbe{
		vmc: vmc,
		Ch:  make(chan struct{}, 1),
	}
}

// ProbeUntilReady attempts SSH connections until one succeeds.
// It blocks until the SSH service is ready or the context is cancelled.
// The Ch channel is closed when the service becomes ready.
// Returns nil on success, ctx.Err() on context cancellation/timeout.
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

// PodmanProbe polls the Podman API until it responds to ping requests.
// Requests are forwarded through gvproxy tunnel to the guest Podman service.
type PodmanProbe struct {
	unixURL string
	Ch      chan struct{}
	once    sync.Once
}

// NewPodmanProbe creates a new PodmanProbe that monitors the given API socket.
// The unixURL can be either a unix:// URL or a raw socket path.
func NewPodmanProbe(unixURL string) *PodmanProbe {
	return &PodmanProbe{
		unixURL: unixURL,
		Ch:      make(chan struct{}, 1),
	}
}

// ProbeUntilReady polls the Podman /libpod/_ping endpoint until it returns HTTP 200.
// It blocks until the service is ready or the context is cancelled.
// The Ch channel is closed when the service becomes ready.
// Returns nil on success, ctx.Err() on context cancellation/timeout.
// TODO: support TSI network
func (p *PodmanProbe) ProbeUntilReady(ctx context.Context) error {
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
