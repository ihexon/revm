package service

import (
	"context"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/vmconfig"

	"github.com/sirupsen/logrus"
)

func PodmanAPIProxy(ctx context.Context, vmc *vmconfig.VMConfig) error {
	if vmc.TSI {
		return nil
	}

	if err := WaitAll(ctx,
		NewGVProxyProbe(vmc.GVPCtlAddr),
	); err != nil {
		return err
	}

	return gvproxy.TunnelHostUnixToGuest(ctx,
		vmc.GVPCtlAddr,
		vmc.PodmanInfo.LocalPodmanProxyAddr,
		vmc.PodmanInfo.GuestPodmanAPIIP,
		vmc.PodmanInfo.GuestPodmanAPIPort)
}

func SendPodmanReady(ctx context.Context, vmc *vmconfig.VMConfig) error {
	if err := WaitAll(ctx, NewPodmanProbe(vmc)); err != nil {
		return err
	}
	logrus.Infof("Podman API ready: %s", vmc.PodmanInfo.LocalPodmanProxyAddr)
	return nil
}
