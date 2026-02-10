package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/vsock"
	"time"

	"github.com/sirupsen/logrus"
)

// WaitAndNotifyReady polls with the given probe until the service is ready,
// then notifies the host via the ignition server's /ready/{service} endpoint.
func WaitAndNotifyReady(ctx context.Context, serviceName string, probe ProbeFunc) error {
	ctx, _ = context.WithTimeoutCause(ctx, 10*time.Second, fmt.Errorf("probe timed out"))
	if err := pollUntilReady(ctx, probe); err != nil {
		return err
	}

	logrus.Infof("%s is ready, notifying ready event to host via vsock", serviceName)

	svc := vsock.NewVSockService()
	defer svc.Close()
	return svc.PostReady(ctx, serviceName)
}
