//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// NetworkConfigStrategy defines the interface for network configuration strategies.
// Different network modes (GVISOR, TSI) implement this interface to configure
// the VM's network stack in their specific way.
type NetworkConfigStrategy interface {
	// Configure sets up network configuration on the given VM.
	// pathMgr is used to get workspace-relative paths for socket files.
	Configure(ctx context.Context, vmc *define.Machine, pathMgr *PathManager) error
}

// GetNetworkStrategy returns the appropriate NetworkConfigStrategy for the given network mode.
// Returns nil if the mode is invalid/unknown.
func GetNetworkStrategy(mode define.VNetMode) NetworkConfigStrategy {
	switch mode {
	case define.GVISOR:
		return &GVisorNetworkConfig{}
	case define.TSI:
		return &TSINetworkConfig{}
	default:
		return nil
	}
}

// GVisorNetworkConfig implements network configuration for gvisor-tap-vsock mode.
// This mode uses gvisor's userspace network stack with vsock communication.
type GVisorNetworkConfig struct{}

// Configure sets up the gvisor-tap-vsock network configuration.
// It creates Unix socket paths for GVProxy control and virtual network communication.
func (g *GVisorNetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *PathManager) error {
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

// TSINetworkConfig implements network configuration for TSI (Transparent Socket Interception) mode.
// TSI mode uses libkrun's built-in network capabilities without external network stack.
type TSINetworkConfig struct{}

// Configure sets up TSI network mode.
// TSI mode doesn't require gvisor network setup, so this is essentially a no-op.
func (t *TSINetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *PathManager) error {
	logrus.Infof("Using TSI network mode (libkrun built-in networking)")
	// TSI mode doesn't need gvisor network setup
	return nil
}
