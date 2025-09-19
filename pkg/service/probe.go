package service

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
)

func testSSH(ctx context.Context, vmc *vmconfig.VMConfig) error {
	cfg := ssh.NewCfg(define.DefaultGuestAddr, vmc.SSHInfo.User, vmc.SSHInfo.Port, vmc.SSHInfo.HostSSHKeyPairFile)
	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	if err = cfg.Connect(ctx, endpoint.Path); err != nil {
		return fmt.Errorf("failed to connect to ssh server: %w", err)
	}
	defer cfg.CleanUp.DoClean()

	return nil
}

func ProbeAndWaitingSSHService(ctx context.Context, vmc *vmconfig.VMConfig) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := testSSH(ctx, vmc); err != nil {
				logrus.Debugf("test ssh service failed with err: %v try again", err.Error())
				continue
			}
			return
		}
	}
}

func testGuestPodmanService(ctx context.Context, listenedAddr string) error {
	addr, err := network.ParseUnixAddr(listenedAddr)
	if err != nil {
		return fmt.Errorf("failed to parse unix socks address: %w", err)
	}
	client := network.NewUnixHTTPClient(addr.Path, 1*time.Second)
	defer client.Close()

	resp, err := client.Get(ctx, "/libpod/_ping")
	if err != nil {
		return fmt.Errorf("failed to ping /libpod/_ping: %w", err)
	}
	defer resp.Body.Close()

	respRead, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Warnf("failed to read response from /_ping: %v", err)
	}

	logrus.Debugf("ping response: %q", string(respRead))

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	default:
		return fmt.Errorf("unexpected response from /_ping: %q", string(respRead))
	}
}

func ProbeAndWaitingPodmanService(ctx context.Context, vmc *vmconfig.VMConfig) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := testGuestPodmanService(ctx, vmc.PodmanInfo.UnixSocksAddr); err != nil {
				logrus.Debugf("test podman service failed with error: %v, try again", err.Error())
				continue
			}
			return
		}
	}
}
