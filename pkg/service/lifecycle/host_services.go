package lifecycle

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/ignition"
	"linuxvm/pkg/service/management"
	"net"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

type HostServices struct {
	vmc *define.Machine
	vmp interfaces.VMMProvider
}

func NewHostServices(vmc *define.Machine, vmp interfaces.VMMProvider) *HostServices {
	return &HostServices{vmc: vmc, vmp: vmp}
}

func (s *HostServices) StartPodmanProxy(ctx context.Context) error {
	if s.vmc.RunMode != define.ContainerMode.String() {
		return nil
	}

	switch s.vmc.VirtualNetworkMode {
	case define.GVISOR:
		_, portStr, _ := net.SplitHostPort(s.vmc.PodmanInfo.GuestPodmanAPIListenAddr)
		port, _ := strconv.ParseUint(portStr, 10, 16)
		logrus.Infof("podman proxy listening in %s, forward to %s", s.vmc.PodmanInfo.HostPodmanProxyAddr, s.vmc.PodmanInfo.GuestPodmanAPIListenAddr)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			s.vmc.GVPCtlAddr,
			s.vmc.PodmanInfo.HostPodmanProxyAddr,
			define.GuestIP,
			uint16(port))
	case define.TSI:
		f := &network.LocalForwarder{
			UnixSockAddr: s.vmc.PodmanInfo.HostPodmanProxyAddr,
			Target:       s.vmc.PodmanInfo.GuestPodmanAPIListenAddr,
			Timeout:      1 * time.Second,
		}
		return f.Run(ctx)
	default:
		return fmt.Errorf("unsupported virtual network mode: %s", s.vmc.VirtualNetworkMode)
	}
}

func (s *HostServices) StartNetworkStack(ctx context.Context) error {
	if s.vmc.VirtualNetworkMode == define.TSI {
		return nil
	}

	logrus.Info("starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, s.vmc)
}

func (s *HostServices) StartIgnitionService(ctx context.Context) error {
	server := ignition.NewServer(s.vmc)
	return server.Start(ctx)
}

func (s *HostServices) StartMachineManagementAPI(ctx context.Context, stopFn func()) error {
	server, err := management.NewServer(s.vmc, stopFn)
	if err != nil {
		return fmt.Errorf("create management server: %w", err)
	}
	return server.Start(ctx)
}

func (s *HostServices) StartVirtualMachine(ctx context.Context) error {
	return s.vmp.Start(ctx)
}
