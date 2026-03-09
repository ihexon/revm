package service

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func startGuestPodmanService(ctx context.Context, vmc *define.Machine) error {
	addr := "tcp://" + vmc.PodmanInfo.GuestPodmanAPIListenAddr //nolint:nosprintfhostport
	cmd := exec.CommandContext(ctx, "podman", "--log-level", logrus.GetLevel().String(), "system", "service", "--time=0", addr)
	cmd.Stdin = nil
	cmd.Stdout = StderrWriter()
	cmd.Stderr = StderrWriter()
	cmd.Env = append(os.Environ(), vmc.PodmanInfo.GuestPodmanRunWithEnvs...)

	logrus.Debugf("podman cmdline %v", cmd.Args)
	return cmd.Run()
}

// StartPodmanAPIServices support TSI/Gvisor network
func StartPodmanAPIServices(ctx context.Context, vmc *define.Machine) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- startGuestPodmanService(ctx, vmc)
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}
