//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig/internal"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// GVisorNetworkConfig implements network configuration for gvisor-tap-vsock mode.
// This mode uses gvisor's userspace network stack with vsock communication.
type GVisorNetworkConfig struct{}

// Configure sets up the gvisor-tap-vsock network configuration.
// It creates Unix socket paths for GVProxy control and virtual network communication.
func (g *GVisorNetworkConfig) Configure(ctx context.Context, vmc *define.VMConfig, pathMgr *internal.PathManager) error {
	logrus.Infof("Configuring gvisor-tap-vsock network mode")

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetGVPCtlAddr(),
	}

	vmc.GVPCtlAddr = unixAddr.String()
	vmc.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetListenAddr())

	// Clean up any existing sockets
	_ = os.Remove(pathMgr.GetGVPCtlAddr())
	_ = os.Remove(pathMgr.GetVNetListenAddr())

	// Ensure parent directory exists
	return os.MkdirAll(filepath.Dir(unixAddr.Path), 0755)
}
