package service

import (
	"context"
	"errors"
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
		Restart:     true,
		MaxRetries:  5,
		RetryDelay:  500 * time.Millisecond,
		StopTimeout: 5 * time.Second,
		Env:         vmc.Podman.GuestPodmanRunWithEnvs,
	})

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		s.Run(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Podman service stopped after exhausting restarts")
	})
	g.Go(func() error {
		return podmanproxy.ServeGuest(ctx, listener, define.GuestPodmanSocket)
	})
	return g.Wait()
}
