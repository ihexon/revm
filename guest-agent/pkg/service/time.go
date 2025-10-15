//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"
	"time"
)

const NTPServer = "time.cloudflare.com"

func SyncRTCTime(ctx context.Context) error {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-ticker.C:
			syncTimeFromNtpServer(ctx)
		}
	}
}

func syncTimeFromNtpServer(ctx context.Context) {
	_ = Busybox.Exec(ctx, "ntpd", "-q", "-n", "-p", NTPServer)
}
