package service

import (
	"context"
	_ "embed"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/define"
	"time"

	"github.com/sirupsen/logrus"
)

func StartGuestPodmanService(ctx context.Context, vmc *define.Machine) error {

	addr := "tcp://" + vmc.PodmanInfo.GuestPodmanAPIListenAddr //nolint:nosprintfhostport

	s := supervisor.New(supervisor.Config{
		Cmd: "podman",
		Args: []string{
			"--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", addr,
		},
		Restart:     true,
		MaxRetries:  5,
		RetryDelay:  500 * time.Millisecond,
		StopTimeout: 5 * time.Second,
		Env:         vmc.PodmanInfo.GuestPodmanRunWithEnvs,
	})

	s.Run(ctx)
	return nil
}
