//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package networkmode

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/service"

	"github.com/sirupsen/logrus"
)

// GVisorMode implements the gvisor-tap-vsock network mode.
// This mode uses an external network stack (gvisor) with vsock communication.
type GVisorMode struct{}

func (g *GVisorMode) StartNetworkStack(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vmc)
}

func (g *GVisorMode) WaitNetworkReady(ctx context.Context, vmc *define.VMConfig) error {
	return service.WaitAll(ctx, service.NewGVProxyProbe(vmc.GVPCtlAddr))
}

func (g *GVisorMode) StartPodmanProxy(ctx context.Context, vmc *define.VMConfig) error {
	// Wait for GVProxy to be ready first
	if err := service.WaitAll(ctx, service.NewGVProxyProbe(vmc.GVPCtlAddr)); err != nil {
		return err
	}

	// Start the tunnel from host Unix socket to guest TCP
	return gvproxy.TunnelHostUnixToGuest(ctx,
		vmc.GVPCtlAddr,
		vmc.PodmanInfo.PodmanProxyAddr,
		vmc.PodmanInfo.GuestPodmanAPIIP,
		vmc.PodmanInfo.GuestPodmanAPIPort)
}

func (g *GVisorMode) WaitPodmanReady(ctx context.Context, vmc *define.VMConfig) error {
	if err := service.WaitAll(ctx, service.NewPodmanProbe(vmc)); err != nil {
		return err
	}
	logrus.Infof("Podman API ready: %s", vmc.PodmanInfo.PodmanProxyAddr)
	return nil
}

func (g *GVisorMode) GetPodmanListenAddr(vmc *define.VMConfig) string {
	// GVISOR mode uses Unix socket proxy
	return vmc.PodmanInfo.PodmanProxyAddr
}

func (g *GVisorMode) String() string {
	return define.GVISOR.String()
}
