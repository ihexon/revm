package vmconfig

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// ============================================================================
// Service Prober Interface
// ============================================================================

// ServiceProber defines the interface for service health checking
type ServiceProber interface {
	// Probe checks if the service is ready
	Probe(ctx context.Context) error
	// Name returns the service name for logging
	Name() string
	// NotifyReady signals that the service is ready by closing the channel
	NotifyReady()
}

// baseProber provides common channel notification logic
type baseProber struct {
	readyChan chan struct{}
	closeOnce func(func())
}

// newBaseProber creates a baseProber from Stage and ServiceType
func newBaseProber(stage *define.Stage, serviceType define.ServiceType) baseProber {
	readyChan, closeOnce := stage.GetReadyChannel(serviceType)
	return baseProber{
		readyChan: readyChan,
		closeOnce: closeOnce,
	}
}

func (b *baseProber) NotifyReady() {
	if b.closeOnce != nil {
		b.closeOnce(func() {
			close(b.readyChan)
		})
	}
}

// unixSocketStatusProber checks if an HTTP endpoint returns 200 OK
type unixSocketStatusProber struct {
	baseProber
	name                string
	unixSocketFile      string
	path                string
	JustTestFileExisted bool
}

func (p *unixSocketStatusProber) Name() string {
	return p.name
}

func (p *unixSocketStatusProber) Probe(ctx context.Context) error {
	if !system.IsPathExist(p.unixSocketFile) {
		return fmt.Errorf("%s socket file %q does not exist", p.name, p.unixSocketFile)
	}

	if p.JustTestFileExisted {
		return nil
	}

	client := network.NewUnixHTTPClient(p.unixSocketFile, 1*time.Second)
	defer client.Close()

	resp, err := client.Get(ctx, p.path)
	if err != nil {
		return fmt.Errorf("failed to request %q: %w", p.path, err)
	}
	defer resp.Body.Close()

	// Drain response body
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		logrus.Debugf("failed to drain response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d from %q", resp.StatusCode, p.path)
	}

	return nil
}

// sshProber checks if SSH service is accessible
// Only contains the minimal fields needed for SSH probing
type sshProber struct {
	baseProber
	gvproxyCtlSocketFile string
	sshKeyPairFile       string
}

func (p *sshProber) Name() string {
	return "Guest SSH Server"
}

func (p *sshProber) Probe(ctx context.Context) error {
	cfg := ssh.NewCfg(
		define.DefaultGuestAddr,
		define.DefaultGuestUser,
		define.DefaultGuestSSHDPort,
		p.sshKeyPairFile,
	)

	if err := cfg.Connect(ctx, p.gvproxyCtlSocketFile); err != nil {
		return fmt.Errorf("failed to connect to ssh server: %w", err)
	}

	defer cfg.CleanUp.DoClean()

	return nil
}

// probeUntilReady runs a continuous probe loop until the service is ready
// or the context is cancelled. When ready, it notifies via the prober's channel.
func probeUntilReady(ctx context.Context, prober ServiceProber, interval time.Duration) {
	if interval == 0 {
		interval = 50 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	serviceName := prober.Name()
	logrus.Debugf("starting probe for %s service", serviceName)

	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("probe for %s cancelled: %v", serviceName, ctx.Err())
			return

		case <-ticker.C:
			if err := prober.Probe(ctx); err != nil {
				logrus.Debugf("%s service not ready: %v", serviceName, err)
				continue
			}

			prober.NotifyReady()
			logrus.Debugf("%s service is ready", serviceName)
			return
		}
	}
}

// waitGVProxyAlive probes GVProxy network backend until it's ready
func waitGVProxyAlive(ctx context.Context, vmc *VMConfig) error {
	addr, err := network.ParseUnixAddr(vmc.NetworkStackBackend)
	if err != nil {
		return err
	}

	prober := &unixSocketStatusProber{
		baseProber:     newBaseProber(&vmc.Stage, define.ServiceGVProxy),
		name:           "gvproxy",
		unixSocketFile: addr.Path,
		// For gvproxy we only need to check if the socket file exists,
		JustTestFileExisted: true,
	}

	probeUntilReady(ctx, prober, 0)

	return nil
}

// waitIgnServerAlive probes Ignition provisioner server until it's ready
func waitIgnServerAlive(ctx context.Context, vmc *VMConfig) error {
	addr, err := network.ParseUnixAddr(vmc.IgnProvisionerAddr)
	if err != nil {
		return err
	}

	prober := &unixSocketStatusProber{
		baseProber:     newBaseProber(&vmc.Stage, define.ServiceIgnServer),
		name:           "Ignition Server",
		unixSocketFile: addr.Path,
		path:           "/healthz",
	}

	probeUntilReady(ctx, prober, 0)
	return nil
}

// waiteHostPodmanForwardAlive probes guest Podman service until it's ready
func waiteHostPodmanForwardAlive(ctx context.Context, vmc *VMConfig) error {
	addr, err := network.ParseUnixAddr(vmc.PodmanInfo.UnixSocksAddr)
	if err != nil {
		return err
	}

	prober := &unixSocketStatusProber{
		baseProber:     newBaseProber(&vmc.Stage, define.ServiceGuestPodman),
		name:           "Guest Podman",
		unixSocketFile: addr.Path,
		path:           "/libpod/_ping",
	}

	probeUntilReady(ctx, prober, 0)
	return nil
}

// waiteSSHTunnelAlive probes guest SSH service until it's ready
func waiteSSHTunnelAlive(ctx context.Context, vmc *VMConfig) error {
	addr, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	prober := &sshProber{
		baseProber:           newBaseProber(&vmc.Stage, define.ServiceGuestSSHServer),
		gvproxyCtlSocketFile: addr.Path,
		sshKeyPairFile:       vmc.SSHInfo.HostSSHKeyPairFile,
	}

	probeUntilReady(ctx, prober, 0)
	return nil
}

func (v *VMConfig) WaitForServices(ctx context.Context, services ...define.ServiceType) error {
	for _, svc := range services {
		readyChan, _ := v.Stage.GetReadyChannel(svc)
		select {
		case <-readyChan:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (v *VMConfig) CloseChannelWhenServiceReady(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer logrus.Infof("gvproxy service available now")
		return waitGVProxyAlive(ctx, v)
	})

	g.Go(func() error {
		return waitIgnServerAlive(ctx, v)
	})

	if v.RunMode == define.ContainerMode.String() {
		g.Go(func() error {
			return waiteHostPodmanForwardAlive(ctx, v)
		})
	}

	g.Go(func() error {
		return waiteSSHTunnelAlive(ctx, v)
	})

	return g.Wait()
}
