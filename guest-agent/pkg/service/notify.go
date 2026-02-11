package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/vsock"
	"time"

	"github.com/sirupsen/logrus"
)

func WaitAndNotifyReady(ctx context.Context, serviceName string, timeout time.Duration, probe ProbeFunc) error {
	ctx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("probe %q timed out after %v", serviceName, timeout))
	defer cancel()

	if err := pollUntilReady(ctx, probe); err != nil {
		return err
	}

	logrus.Infof("%s is ready, notifying ready event to host via vsock", serviceName)

	svc := vsock.NewVSockService()
	defer svc.Close()
	return svc.PostReady(ctx, serviceName)
}
