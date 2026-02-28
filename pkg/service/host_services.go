package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/network"
	"net"
	"os"
	"os/signal"
	"runtime"
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
	event.Emit(event.StartPodmanProxyServer)
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
	event.Emit(event.StartVirtualNetwork)
	if s.vmp.GetVMConfigure().VirtualNetworkMode == define.TSI {
		return nil
	}

	logrus.Info("Starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, s.vmp.GetVMConfigure())
}

func (s *HostServices) StartIgnitionService(ctx context.Context) error {
	event.Emit(event.StartIgnitionServer)
	server := NewIgnServer(s.vmp.GetVMConfigure())
	return server.Start(ctx)
}

func (s *HostServices) StartMachineManagementAPI(ctx context.Context) error {
	event.Emit(event.StartManagementAPIServer)
	return s.vmp.StartVMCtlServer(ctx)
}

func (s *HostServices) StartVirtualMachine(ctx context.Context) error {
	event.Emit(event.StartVirtualMachine)

	errChan := make(chan error, 1)
	go func() {
		// Pin all libkrun CGo calls to a single OS thread: krun_create_ctx,
		// krun_add_virtiofs2, krun_start_enter, etc. must not migrate
		// threads — on Linux/KVM this causes GC heap corruption.
		runtime.LockOSThread()
		if err := s.vmp.Create(ctx); err != nil {
			errChan <- fmt.Errorf("create virtual machine from libkrun builder fail: %v", err)
			return
		}
		errChan <- s.vmp.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func (s *HostServices) ExitVirtualMachineWhenSomethingHappened(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.vmp.GetVMConfigure().StopCh:
			return fmt.Errorf("stopCh closed, shutdown machine down")
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
					s.vmp.GetVMConfigure().StopOnce.Do(func() { close(s.vmp.GetVMConfigure().StopCh) })
					logrus.Warn("parent process exited, shutting down...")
					return fmt.Errorf("parent process exited, shutting down")
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
			s.vmp.GetVMConfigure().StopOnce.Do(func() {
				close(s.vmp.GetVMConfigure().StopCh)
			})
			logrus.Warn("received SIGTERM/SIGINT, shutting down...")
			return fmt.Errorf("received SIGTERM/SIGINT, shutting down")
		}
	})

	return g.Wait()
}
