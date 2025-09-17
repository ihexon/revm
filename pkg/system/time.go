//go:build (linux || darwin) && (arm64 || amd64)

package system

import (
	"context"
	"os/exec"
	"time"

	"github.com/sirupsen/logrus"
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
	for i := 0; i < 3; i++ {
		if err := exec.CommandContext(ctx, GetGuestLinuxUtilsBinPath("busybox.static"), "ntpd", "-q", "-n", "-p", NTPServer).Run(); err != nil {
			logrus.Warnf("failed to sync time: %v, try again", err)
			continue
		}
		break
	}
}
