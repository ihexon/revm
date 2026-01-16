package probes

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type ServiceProber interface {
	ProbeUntilReady(ctx context.Context) error
	WaitUntilReady(ctx context.Context)
}

const (
	defaultProbeTimeout  = 50 * time.Millisecond
	defaultProbeInterval = 50 * time.Millisecond
)

type GVProxyService struct {
	UnixSocketPath string
	Ch             chan struct{}
	once           sync.Once
}

func NewGVProxyService(unixAddress string) *GVProxyService {
	return &GVProxyService{
		UnixSocketPath: unixAddress,
		Ch:             make(chan struct{}, 1),
	}
}

func (g *GVProxyService) ProbeUntilReady(ctx context.Context) error {
	logrus.Warnf("not implemented yet, unix address: %s", g.UnixSocketPath)
	g.once.Do(func() { close(g.Ch) })
	return nil
}
func (g *GVProxyService) WaitUntilReady(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-g.Ch:
		return
	}
}

type VMConfigProvisionerServer struct {
	unixAddress string
	Ch          chan struct{}
	once        sync.Once
}

func NewVMConfigProvisionerServer(unixAddress string) *VMConfigProvisionerServer {
	return &VMConfigProvisionerServer{
		unixAddress: unixAddress,
		Ch:          make(chan struct{}, 1),
	}
}

// ProbeUntilReady checks if the VMConfigProvisionerServer is ready
func (v *VMConfigProvisionerServer) ProbeUntilReady(ctx context.Context) error {
	client := network.NewUnixHTTPClient(v.unixAddress, defaultProbeTimeout)
	defer client.Close()

	ticker := time.NewTicker(defaultProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(ctx, "/healthz") // nolint:bodyclose
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				logrus.Warnf("get VMConfigProvisionerServer /healthz with status code: %d, try again", resp.StatusCode)
				client.CloseResponse(resp)
				continue
			}

			client.CloseResponse(resp)
			// close the channel to notify the service is ready
			v.once.Do(func() { close(v.Ch) })
			return nil
		}
	}
}

func (v *VMConfigProvisionerServer) WaitUntilReady(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-v.Ch:
		return
	}
}

type GuestSSHService struct {
	// gvproxyCtlSocketPath this tunnel is required to connect to the guest SSH service
	//
	// see the code example: https://github.com/containers/gvisor-tap-vsock/blob/main/cmd/ssh-over-vsock/main.go
	gvproxyCtlSocketPath string
	sshPrivateKeyPath    string
	Ch                   chan struct{}
	once                 sync.Once
}

func NewGuestSSHService(gvproxyCtlSocketPath, sshPrivateKeyPath string) *GuestSSHService {
	return &GuestSSHService{
		gvproxyCtlSocketPath: gvproxyCtlSocketPath,
		sshPrivateKeyPath:    sshPrivateKeyPath,
		Ch:                   make(chan struct{}, 1),
	}
}

func (s *GuestSSHService) ProbeUntilReady(ctx context.Context) error {
	// ssh.NewClient will connect to the SSH server and close the connection immediately
	cfg := ssh.NewClientConfig(define.DefaultGuestAddr, uint16(define.DefaultGuestSSHDPort), define.DefaultGuestUser, s.sshPrivateKeyPath)
	cfg.WithGVProxySocket(s.gvproxyCtlSocketPath)
	cfg.WithDialTimeout(defaultProbeTimeout)

	ticker := time.NewTicker(defaultProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			client, err := ssh.NewClient(ctx, cfg)
			if err != nil {
				logrus.Warnf("probe the ssh server with %s has error: %v", s.gvproxyCtlSocketPath, err)
				continue
			}

			if err = client.Close(); err != nil {
				logrus.Warnf("failed to close ssh client: %v", err)
			}
			s.once.Do(func() { close(s.Ch) })
			return nil
		}
	}
}

func (s *GuestSSHService) WaitUntilReady(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-s.Ch:
		return
	}
}

type PodmanService struct {
	// This is the Podman API address. All requests to this API address will be forwarded to the
	// Podman API in the guest system. This forwarding is accomplished through a tunnel provided by GVProxy.
	apiSocketPath string
	Ch            chan struct{}
	once          sync.Once
}

func NewPodmanService(socketPath string) *PodmanService {
	return &PodmanService{
		apiSocketPath: socketPath,
		Ch:            make(chan struct{}, 1),
	}
}

func (p *PodmanService) ProbeUntilReady(ctx context.Context) error {
	client := network.NewUnixHTTPClient(p.apiSocketPath, 50*time.Millisecond)
	defer client.Close()

	ticker := time.NewTicker(defaultProbeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(ctx, "/libpod/_ping") // nolint:bodyclose
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				logrus.Warnf("ping Podman API with return code: %d, try again", resp.StatusCode)
				client.CloseResponse(resp)
				continue
			}

			client.CloseResponse(resp)
			p.once.Do(func() { close(p.Ch) })
			return nil
		}
	}
}

func (p *PodmanService) WaitUntilReady(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-p.Ch:
		return
	}
}
