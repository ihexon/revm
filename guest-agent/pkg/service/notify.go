package service

import (
	"context"
	"guestAgent/pkg/vsock"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultLocalProbeInterval = 10 * time.Millisecond

// WaitAndNotifyReady probes a local TCP address until it's ready,
// then notifies the host via the ignition server's /ready/{service} endpoint.
func WaitAndNotifyReady(ctx context.Context, serviceName string, addr string) error {
	if err := probeLocalTCP(ctx, addr, defaultLocalProbeInterval); err != nil {
		return err
	}
	logrus.Infof("%s is listening locally, notifying host", serviceName)

	svc := vsock.NewVSockService()
	defer svc.Close()
	return svc.PostReady(ctx, serviceName)
}
