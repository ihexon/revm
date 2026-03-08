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
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type HostServices struct {
	vmc *define.Machine
	vmp interfaces.VMMProvider
}

func NewHostServices(vmc *define.Machine, vmp interfaces.VMMProvider) *HostServices {
	return &HostServices{vmc: vmc, vmp: vmp}
}

func (s *HostServices) StartPodmanProxy(ctx context.Context) error {
	if s.vmc.RunMode == define.RootFsMode.String() {
		return nil
	}

	switch s.vmc.VirtualNetworkMode {
	case define.GVISOR:
		_, portStr, _ := net.SplitHostPort(s.vmc.PodmanInfo.GuestPodmanAPIListenAddr)
		port, _ := strconv.ParseUint(portStr, 10, 16)
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

	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, s.vmc)
}

func (s *HostServices) StartIgnitionService(ctx context.Context) error {
	server := ignition.NewServer(s.vmc)
	return server.Start(ctx)
}

func (s *HostServices) StartMachineManagementAPI(ctx context.Context, stopFn func()) error {
	server, err := management.NewServer(s.vmc, stopFn)
	if err != nil {
		return err
	}
	return server.Start(ctx)
}

func (s *HostServices) StartVirtualMachine(ctx context.Context) error {
	return s.vmp.Start(ctx)
}

func (s *HostServices) ExitVirtualMachineWhenSomethingHappened(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.vmc.StopCh:
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
					logrus.Infof("parent process exit, exit virtual machine")
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
