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

	rd := service.NewServiceReadiness(vmc)
	svc := vsock.NewVSockService()
	defer svc.Close()

	// Check each service sequentially. PostReady failures are logged but not fatal.
	if rd.IsSSHReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNameSSH); err != nil {
			logrus.Debugf("[service] PostReady(ssh) failed: %v", err)
		}
	} else {
		logrus.Debugf("[service] guest ssh not ready")
	}

	if rd.IsPodmanReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNamePodman); err != nil {
			logrus.Debugf("[service] PostReady(podman) failed: %v", err)
		}
	} else {
		logrus.Debugf("[service] guest podman not ready")
	}

	if rd.IsInterfaceReady(ctx) {
		if err := svc.PostReady(ctx, define.ServiceNameGuestNetwork); err != nil {
			logrus.Debugf("[service] PostReady(guest-network) failed: %v", err)
		}
	} else {
		logrus.Debugf("[service] guest interface not ready")
	}

	logrus.Debugf("[service] all guest services readiness checks completed")
	return nil
}
