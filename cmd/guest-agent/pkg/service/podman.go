package service

import (
	"context"
	_ "embed"
	"fmt"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/define"
	"time"

	"github.com/sirupsen/logrus"
)

func StartGuestPodmanService(ctx context.Context, vmc *define.Machine) error {
	s := supervisor.New(supervisor.Config{
		Cmd: "podman",
		Args: []string{
			"--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", fmt.Sprintf("tcp://%s", vmc.PodmanInfo.GuestPodmanAPIListenAddr),
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
