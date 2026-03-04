//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"
	"time"
)

var servers = []string{
	"asia.pool.ntp.org",
	"tw.pool.ntp.org",
	"north-america.pool.ntp.org",
	"jp.pool.ntp.org",
}

func SyncRTCTime(ctx context.Context) error {
	return syncTimeFromNtpServerForever(ctx, 10)
}

func syncTimeFromNtpServerForever(ctx context.Context, sleepTimeInSecond uint64) error {
	for {
		for i := range servers {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-time.After(time.Duration(sleepTimeInSecond) * time.Second):
				_ = ExecOutput(ctx, nil, StderrWriter(), "ntpd", "-q", "-n", "-p", servers[i])
			}
		}
	}
}
