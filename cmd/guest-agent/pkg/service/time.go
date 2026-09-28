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
		// Some supported BusyBox builds omit ntpd. The guest clock is still
		// initialized by the VMM, so an optional sync failure is diagnostic only.
		logrus.Debugf("guest clock sync unavailable: %v", err)
	}
	return nil
}
