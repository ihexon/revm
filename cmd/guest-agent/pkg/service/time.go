//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"
	"guestAgent/pkg/supervisor"
	"time"
)

func SyncRTCTime(ctx context.Context) error {
	args := []string{"ntpd", "-q", "-p", "pool.ntp.org"}

	sv := supervisor.New(supervisor.Config{
		Name:       "ntpd",
		Cmd:        BusyboxPath(),
		Args:       args,
		Stderr:     StderrWriter(),
		Stdout:     StderrWriter(),
		Restart:    true,
		RetryDelay: 1 * time.Minute,
	})
	sv.Run(ctx)
	return nil
}
