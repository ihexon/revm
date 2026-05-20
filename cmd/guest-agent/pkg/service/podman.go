package service

import (
	"context"
	_ "embed"
	"fmt"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/protocol"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

func StartGuestPodmanService(ctx context.Context, vmc *protocol.GuestSpec) error {
	// podman need /var/tmp exist, we make sure the directory exist
	if err := os.MkdirAll("/var/tmp", 0755); err != nil {
		return fmt.Errorf("failed to create /var/tmp for podman: %w", err)
	}

	s := supervisor.New(supervisor.Config{
		Cmd: "podman",
		Args: []string{
			"--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", fmt.Sprintf("tcp://%s", vmc.Podman.GuestPodmanAPIListenAddr),
		},
		Restart:     true,
		MaxRetries:  5,
		RetryDelay:  500 * time.Millisecond,
		StopTimeout: 5 * time.Second,
		Env:         vmc.Podman.GuestPodmanRunWithEnvs,
	})

	s.Run(ctx)
	return nil
}
