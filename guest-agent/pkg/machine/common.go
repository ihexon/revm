package machine

import (
	"context"
	"fmt"
	"guestAgent/pkg/service"
	"guestAgent/pkg/vsock"
	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type Machine define.VMConfig

func (m *Machine) GetVirtualNetworkType() define.VNetMode {
	if m.VirtualNetworkMode == define.TSI.String() {
		return define.TSI
	}

	return define.GVISOR
}

func WaitGuestServiceReady(ctx context.Context, vmc *define.VMConfig) error {
	ctx, cancel := context.WithTimeoutCause(ctx, define.DefaultProbeTimeout, fmt.Errorf("readiness timed out after %v", define.DefaultProbeTimeout))
	defer cancel()

	svc := vsock.NewVSockService()
	defer svc.Close()

	rd := service.NewServiceReadiness(vmc)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if rd.IsSSHReady(gctx) {
			if err := svc.PostReady(gctx, define.ServiceNameSSH); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("[service] guest ssh not ready")
	})

	g.Go(func() error {
		if rd.IsPodmanReady(gctx) {
			if err := svc.PostReady(gctx, define.ServiceNamePodman); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("[service] guest podman not ready")
	})

	g.Go(func() error {
		if rd.IsInterfaceReady(gctx) {
			return nil
		}
		return fmt.Errorf("[service] guest interface not ready")
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if err == nil {
			logrus.Infof("[service] all guest services are ready")
		}
		return err
	}
}
