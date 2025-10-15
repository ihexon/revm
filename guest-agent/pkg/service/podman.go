package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func startGuestPodmanService(ctx context.Context) error {
	addr := fmt.Sprintf("tcp://%s:%d", define.UnspecifiedAddress, define.DefaultGuestPodmanAPIPort) //nolint:nosprintfhostport
	cmd := exec.CommandContext(ctx, "podman", "system", "service", "--time=0", addr)
	cmd.Stdin = nil

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	logrus.Debugf("podman service cmdline: %q", cmd.Args)
	return cmd.Run()
}

func StartPodmanAPIServices(ctx context.Context) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- startGuestPodmanService(ctx)
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}
