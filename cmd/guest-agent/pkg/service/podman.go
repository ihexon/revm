package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/supervisor"
	guestvsock "guestAgent/pkg/vsock"
	"linuxvm/pkg/define"
	"linuxvm/pkg/podmanproxy"
	"linuxvm/pkg/protocol"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func StartGuestPodmanService(ctx context.Context, vmc *protocol.GuestSpec) error {
	if err := os.MkdirAll("/var/tmp", 0755); err != nil {
		return fmt.Errorf("create Podman temporary directory: %w", err)
	}
	dir := filepath.Dir(define.GuestPodmanSocket)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	listener, err := guestvsock.ListenHost(define.GuestPodmanPort)
	if err != nil {
		return fmt.Errorf("listen Podman host channel: %w", err)
	}
	defer listener.Close()

	// Keep every replacement socket private, including after a service restart.
	// Configuration remains argv and is never interpolated into shell text.
	s := supervisor.New(supervisor.Config{
		Name: "podman", Cmd: "/bin/sh",
		Args: []string{
			"-c", "umask 077; exec \"$@\"", "podman-service",
			"podman", "--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", "unix://" + define.GuestPodmanSocket,
		},
		StopTimeout: time.Second,
		Env:         vmc.Podman.GuestPodmanRunWithEnvs,
	})

	// Stop containers while the engine is still available, then stop the API.
	// Container state remains on the volume; no container survives VM shutdown.
	engineCtx, stopEngine := context.WithCancel(context.WithoutCancel(ctx))
	defer stopEngine()
	g, engineCtx := errgroup.WithContext(engineCtx)
	g.Go(func() error { return s.Run(engineCtx) })
	g.Go(func() error { return podmanproxy.ServeGuest(engineCtx, listener, define.GuestPodmanSocket) })
	done := make(chan error, 1)
	go func() { done <- g.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := ExecNoOutput(stopCtx, "podman", "--remote", "--url", "unix://"+define.GuestPodmanSocket, "stop", "--all", "--time", "2"); err != nil {
			logrus.Warnf("stop containers: %v", err)
		}
		stopEngine()
		<-done
		return ctx.Err()
	}
}
