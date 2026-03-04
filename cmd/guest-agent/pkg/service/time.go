//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

const NTPServer = "time.cloudflare.com"

func SyncRTCTime(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-ticker.C:
			if err := syncTimeFromNtpServer(ctx); err != nil {
				logrus.Warnf("sync time error: %v", err)
			}
		}
	}
}

func syncTimeFromNtpServer(ctx context.Context) error {
	return ExecOutput(ctx, nil, StderrWriter(), "ntpd", "-q", "-n", "-p", NTPServer)
}
