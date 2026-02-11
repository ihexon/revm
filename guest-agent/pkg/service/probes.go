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

type ProbeFunc func(ctx context.Context) error

func ProbeSSHFn(_ *define.VMConfig, sshKeyFile string) ProbeFunc {
	return func(ctx context.Context) error {
		client, err := ssh_v2.Dial(ctx, fmt.Sprintf("%s:%d", define.LocalHost, define.GuestSSHServerPort),
			ssh_v2.WithPrivateKey(sshKeyFile),
			ssh_v2.WithUser(define.DefaultGuestUser),
		)
		if err != nil {
			return err
		}

		defer client.Close()
		return client.RunWith(ctx, define.BuiltinBusybox, nil, io.Discard, io.Discard)
	}
}

func ProbePodmanFn(vmc *define.VMConfig) ProbeFunc {
	return func(ctx context.Context) error {
		client := network.NewTCPClient(fmt.Sprintf("%s:%d", define.LocalHost, define.GuestPodmanAPIPort))
		defer client.Close()

		resp, err := client.NewRequest(http.MethodGet, "_ping").Do(ctx)
		if err != nil {
			return err
		}
		defer network.CloseResponse(resp)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("podman _ping returned %d", resp.StatusCode)
		}


		return nil
	}
}

// pollUntilReady calls probe repeatedly until it succeeds or ctx is cancelled.
func pollUntilReady(ctx context.Context, probe ProbeFunc) error {
	ticker := time.NewTicker(define.DefaultTimeTicker)
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
				logrus.Warnf("probe failed with: %v", err)
			}
		}
	}
}