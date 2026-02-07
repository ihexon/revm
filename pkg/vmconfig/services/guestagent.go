//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package services

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/static_resources"
	"linuxvm/pkg/vmconfig/internal"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// GuestAgentConfigurator handles guest agent configuration.
type GuestAgentConfigurator struct {
	pathMgr *internal.PathManager
}

// NewGuestAgentConfigurator creates a new guest agent configurator.
func NewGuestAgentConfigurator(pathMgr *internal.PathManager) *GuestAgentConfigurator {
	return &GuestAgentConfigurator{pathMgr: pathMgr}
}

// Configure sets up the guest agent including ignition server and environment variables.
func (g *GuestAgentConfigurator) Configure(ctx context.Context, vmc *define.VMConfig) error {
	if vmc.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	port, err := network.GetAvailablePort(62234)
	if err != nil {
		return err
	}
	logrus.Infof("ignition server port will listen in: %d", port)

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   g.pathMgr.GetIgnAddr(),
	}

	if err = os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	if err = os.Remove(unixUSL.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	vmc.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenSockAddr: unixUSL.String(),
	}

	if vmc.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	// inject user-given envs to guest-agent
	var finalEnv []string
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "TMPDIR=/tmp")

	// In the virtualNetwork, HOST_DOMAIN is the domain name of the host, and the guest can access network resources on the host through HOST_DOMAIN.
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomainInGVPNet))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	guestAgentFilePath := filepath.Join(vmc.RootFS, ".bin", "guest-agent")

	if err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(guestAgentFilePath, static_resources.GuestAgentBytes, 0755); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	vmc.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}
