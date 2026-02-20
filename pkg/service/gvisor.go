//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	"linuxvm/pkg/gvproxy"

	"github.com/sirupsen/logrus"
)

// GVisorMode implements the gvisor-tap-vsock network mode.
// This mode uses an external network stack (gvisor) with vsock communication.
type GVisorMode struct{}

func (g *GVisorMode) StartNetworkStack(ctx context.Context, vmc *define.Machine) error {
	event.Emit(event.StartVirtualNetwork)
	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vmc)
}

func (g *GVisorMode) StartPodmanProxy(ctx context.Context, vmc *define.Machine) error {
	event.Emit(event.StartPodmanProxyServer)
	return gvproxy.TunnelHostUnixToGuest(ctx,
		vmc.GVPCtlAddr,
		vmc.PodmanInfo.PodmanProxyAddr,
		vmc.PodmanInfo.GuestPodmanAPIIP,
		vmc.PodmanInfo.GuestPodmanAPIPort)
}
