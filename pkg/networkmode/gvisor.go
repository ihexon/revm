//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package networkmode

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"

	"github.com/sirupsen/logrus"
)

// GVisorMode implements the gvisor-tap-vsock network mode.
// This mode uses an external network stack (gvisor) with vsock communication.
type GVisorMode struct{}

func (g *GVisorMode) StartNetworkStack(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vmc)
}

func (g *GVisorMode) StartPodmanProxy(ctx context.Context, vmc *define.VMConfig) error {
	return gvproxy.TunnelHostUnixToGuest(ctx,
		vmc.GVPCtlAddr,
		vmc.PodmanInfo.PodmanProxyAddr,
		vmc.PodmanInfo.GuestPodmanAPIIP,
		vmc.PodmanInfo.GuestPodmanAPIPort)
}

func (g *GVisorMode) GetPodmanListenAddr(vmc *define.VMConfig) string {
	return vmc.PodmanInfo.PodmanProxyAddr
}

func (g *GVisorMode) String() string {
	return define.GVISOR.String()
}
