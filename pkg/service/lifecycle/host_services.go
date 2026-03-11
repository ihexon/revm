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

type HostServices interface {
	StartPodmanProxy(ctx context.Context) error
	StartNetworkStack(ctx context.Context) error
	StartIgnitionService(ctx context.Context) error
	StartMachineManagementAPI(ctx context.Context) error
	StartVirtualMachine(ctx context.Context) error
	StopVirtualMachine() error
}

type Service struct {
	vmp interfaces.VMMProvider
}

func NewHostServices(vmp interfaces.VMMProvider) *Service {
	return &Service{vmp: vmp}
}

func (s *Service) StartPodmanProxy(ctx context.Context) error {
	vmc := s.vmp.GetVMConfig()
	if vmc.RunMode != define.ContainerMode.String() {
		return nil
	}

	switch vmc.VirtualNetworkMode {
	case define.GVISOR:
		_, portStr, _ := net.SplitHostPort(vmc.PodmanInfo.GuestPodmanAPIListenAddr)
		port, _ := strconv.ParseUint(portStr, 10, 16)
		logrus.Infof("podman proxy listening in %s, forward to %s", vmc.PodmanInfo.HostPodmanProxyAddr, vmc.PodmanInfo.GuestPodmanAPIListenAddr)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			vmc.GVPCtlAddr,
			vmc.PodmanInfo.HostPodmanProxyAddr,
			define.GuestIP,
			uint16(port))
	case define.TSI:
		f := &network.LocalForwarder{
			UnixSockAddr: vmc.PodmanInfo.HostPodmanProxyAddr,
			Target:       vmc.PodmanInfo.GuestPodmanAPIListenAddr,
			Timeout:      1 * time.Second,
		}
		return f.Run(ctx)
	default:
		return fmt.Errorf("unsupported virtual network mode: %s", vmc.VirtualNetworkMode)
	}
}

func (s *Service) StartNetworkStack(ctx context.Context) error {
	vmc := s.vmp.GetVMConfig()
	if vmc.VirtualNetworkMode == define.TSI {
		return nil
	}

	logrus.Info("starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vmc)
}

func (s *Service) StartIgnitionService(ctx context.Context) error {
	server := ignition.NewServer(s.vmp.GetVMConfig())
	return server.Start(ctx)
}

func (s *Service) StartMachineManagementAPI(ctx context.Context) error {
	server, err := management.NewServer(s.vmp)
	if err != nil {
		return fmt.Errorf("create management server: %w", err)
	}
	return server.Start(ctx)
}

func (s *Service) StartVirtualMachine(ctx context.Context) error {
	return s.vmp.Start(ctx)
}

func (s *Service) StopVirtualMachine() error {
	return s.vmp.Stop()
}
