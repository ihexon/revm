//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package services

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig/internal"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// PodmanConfigurator handles Podman service configuration.
type PodmanConfigurator struct {
	pathMgr *internal.PathManager
}

// NewPodmanConfigurator creates a new Podman configurator.
func NewPodmanConfigurator(pathMgr *internal.PathManager) *PodmanConfigurator {
	return &PodmanConfigurator{pathMgr: pathMgr}
}

// Configure sets up Podman API configuration including proxy settings.
func (p *PodmanConfigurator) Configure(ctx context.Context, vmc *define.VMConfig, ifUsingSystemProxy bool) error {
	var envs []string

	if ifUsingSystemProxy {
		logrus.Warnf("your system proxy must support CONNECT method")
		proxyInfo, err := network.GetAndNormalizeSystemProxy()
		if err != nil {
			return fmt.Errorf("failed to get and normalize system proxy: %w", err)
		}

		if proxyInfo.HTTP != nil {
			envs = append(envs, fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port))
		}

		if proxyInfo.HTTPS != nil {
			envs = append(envs, fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port))
		}
	}

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   p.pathMgr.GetPodmanListenAddr(),
	}

	vmc.PodmanInfo = define.PodmanInfo{
		PodmanProxyAddr:    podmanProxyAddr.String(),
		GuestPodmanAPIIP:   define.GuestIP,
		GuestPodmanAPIPort: define.GuestPodmanAPIPort,
		Envs:               envs,
	}

	if err := os.MkdirAll(filepath.Dir(podmanProxyAddr.Path), 0755); err != nil {
		return err
	}

	return nil
}
