package service

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh_v2"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultProbeInterval = 10 * time.Millisecond

type ProbeFunc func(ctx context.Context) error

// ProbeSSHFn returns a probe that connects and verifies the SSH-2.0 banner.
func ProbeSSHFn(vmc *define.VMConfig, sshKeyFile string) ProbeFunc {
	return func(ctx context.Context) error {
		logrus.Debugf("probe ssh...")
		client, err := ssh_v2.Dial(ctx, fmt.Sprintf("%s:%d", define.GuestIP, define.GuestSSHServerPort),
			ssh_v2.WithPrivateKey(sshKeyFile),
			ssh_v2.WithUser(define.DefaultGuestUser),
		)
		if err != nil {
			return err
		}

		if err = client.RunWith(ctx, define.BuiltinBusybox, nil, io.Discard, io.Discard); err != nil {
			_ = client.Close()
			return err
		}

		_ = client.Close()
		return nil
	}
}

func ProbePodmanFn(_ *define.VMConfig) ProbeFunc {
	return func(ctx context.Context) error {
		logrus.Debugf("probe podman...")
		client := network.NewTCPClient(fmt.Sprintf("%s:%d", define.GuestIP, define.GuestPodmanAPIPort))
		resp, err := client.NewRequest(http.MethodGet, "_ping").Do(ctx)
		if err != nil {
			_ = client.Close()
			return err
		}
		if resp.StatusCode != http.StatusOK {
			network.CloseResponse(resp)
			_ = client.Close()
		}

		network.CloseResponse(resp)
		_ = client.Close()
		return nil
	}
}

// pollUntilReady calls probe repeatedly until it succeeds or ctx is cancelled.
func pollUntilReady(ctx context.Context, probe ProbeFunc) error {
	ticker := time.NewTicker(defaultProbeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			errCh := make(chan error, 1)
			go func() {
				errCh <- probe(ctx)
			}()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-errCh:
				if err == nil {
					return nil
				}
			}
		}
	}
}
