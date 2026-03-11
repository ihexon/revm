package machine

import (
	"context"
	"fmt"
	"guestAgent/pkg/service"
	"guestAgent/pkg/vsock"
	"linuxvm/pkg/define"

	"github.com/sirupsen/logrus"
)

type Machine define.Machine

func (m *Machine) GetVirtualNetworkType() define.VNetMode {
	if m.VirtualNetworkMode == define.TSI {
		return define.TSI
	}

	return define.GVISOR
}

func WaitGuestServiceReady(ctx context.Context, vmc *define.Machine) error {
	ctx, cancel := context.WithTimeoutCause(ctx, define.DefaultProbeTimeout, fmt.Errorf("readiness timed out after %v", define.DefaultProbeTimeout))
	defer cancel()

	svc := vsock.NewVSockService()
	defer svc.Close()

	rd := service.NewServiceReadiness(vmc)

	if rd.IsSSHReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNameSSH); err != nil {
			return fmt.Errorf("post ssh ready: %w", err)
		}
	}

	if rd.IsPodmanReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNamePodman); err != nil {
			return fmt.Errorf("post podman ready: %w", err)
		}
	}

	if rd.IsInterfaceReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNameGuestNetwork); err != nil {
			return fmt.Errorf("post guest network ready: %w", err)
		}
	}

	logrus.Debugf("[service] all guest services are ready")
	return nil
}
