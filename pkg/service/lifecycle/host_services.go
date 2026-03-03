package lifecycle

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/ignition"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type HostServices struct {
	vmp interfaces.VMMProvider
}

func NewHostServices(vmp interfaces.VMMProvider) *HostServices {
	return &HostServices{vmp: vmp}
}

func (s *HostServices) StartPodmanProxy(ctx context.Context) error {
	if s.vmp.GetVMConfigure().RunMode == define.RootFsMode.String() {
		return nil
	}

	switch s.vmp.GetVMConfigure().VirtualNetworkMode {
	case define.GVISOR:
		_, portStr, _ := net.SplitHostPort(s.vmp.GetVMConfigure().PodmanInfo.GuestPodmanAPIListenAddr)
		port, _ := strconv.ParseUint(portStr, 10, 16)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			s.vmp.GetVMConfigure().GVPCtlAddr,
			s.vmp.GetVMConfigure().PodmanInfo.HostPodmanProxyAddr,
			define.GuestIP,
			uint16(port))
	case define.TSI:
		f := &network.LocalForwarder{
			UnixSockAddr: s.vmp.GetVMConfigure().PodmanInfo.HostPodmanProxyAddr,
			Target:       s.vmp.GetVMConfigure().PodmanInfo.GuestPodmanAPIListenAddr,
			Timeout:      1 * time.Second,
		}
		return f.Run(ctx)
	default:
		return fmt.Errorf("unsupported virtual network mode: %s", s.vmp.GetVMConfigure().VirtualNetworkMode)
	}
}

func (s *HostServices) StartNetworkStack(ctx context.Context) error {
	if s.vmp.GetVMConfigure().VirtualNetworkMode == define.TSI {
		return nil
	}

	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, s.vmp.GetVMConfigure())
}

func (s *HostServices) StartIgnitionService(ctx context.Context) error {
	server := ignition.NewServer(s.vmp.GetVMConfigure())
	return server.Start(ctx)
}

func (s *HostServices) StartMachineManagementAPI(ctx context.Context, stopFn func()) error {
	return s.vmp.StartVMCtlServer(ctx, stopFn)
}

func (s *HostServices) StartVirtualMachine(ctx context.Context) error {
	if err := s.vmp.Create(ctx); err != nil {
		return fmt.Errorf("create virtual machine from libkrun builder fail: %v", err)
	}
	return s.vmp.Start(ctx)
}

func (s *HostServices) ExitVirtualMachineWhenSomethingHappened(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.vmp.GetVMConfigure().StopCh:
			return define.ErrStopChTrigger
		}
	})

	g.Go(func() error {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if os.Getppid() == 1 {
					return define.ErrParentProcessExit
				}
			}
		}
	})

	g.Go(func() error {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)
		defer signal.Stop(sigCh)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sigCh:
			return define.ErrSigTerm
		}
	})

	return g.Wait()
}
