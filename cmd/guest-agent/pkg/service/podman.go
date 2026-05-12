package service

import (
	"context"
	_ "embed"
	"fmt"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/protocol"
	"time"

	"github.com/sirupsen/logrus"
)

func StartGuestPodmanService(ctx context.Context, vmc *protocol.GuestSpec) error {
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
