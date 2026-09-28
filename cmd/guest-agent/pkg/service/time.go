//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"

	"github.com/sirupsen/logrus"
)

func SyncRTCTime(ctx context.Context) error {
	if err := ExecNoOutput(ctx, "ntpd", "-q", "-p", "pool.ntp.org"); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Time synchronization is optional. The guest clock is initialized by the
		// VMM, so an unavailable ntpd package is diagnostic only.
		logrus.Debugf("guest clock sync unavailable: %v", err)
	}
	return nil
}
