//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

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

func (v *VM) configureNetwork(ctx context.Context, mode define.VNetMode, pathMgr *PathManager) error {
	strategy := GetNetworkStrategy(mode)
	if strategy == nil {
		return fmt.Errorf("invalid network mode: %s", mode)
	}
	v.VirtualNetworkMode = mode
	return strategy.Configure(ctx, &v.Machine, pathMgr)
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

	// On Linux, use unix:// (stream socket for QemuProtocol).
	// On macOS, use unixgram:// (datagram socket for VfkitProtocol).
	if runtime.GOOS == "linux" {
		vmc.GVPVNetAddr = fmt.Sprintf("unix://%s", pathMgr.GetVNetListenAddr())
	} else {
		vmc.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetListenAddr())
	}

	// Clean up any existing sockets
	_ = os.Remove(pathMgr.GetGVPCtlAddr())
	_ = os.Remove(pathMgr.GetVNetListenAddr())

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}

	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	vmc.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.UnspecifiedAddress, strconv.FormatUint(port, 10))
	return nil
}

// TSINetworkConfig implements network configuration for TSI (Transparent Socket Interception) mode.
// TSI mode uses libkrun's built-in network capabilities without external network stack.
type TSINetworkConfig struct{}

// Configure sets up TSI network mode.
// TSI mode doesn't require gvisor network setup, but we record the host-accessible
// SSH address since guest ports are directly reachable via libkrun.
func (t *TSINetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *PathManager) error {
	logrus.Infof("Using TSI network mode (libkrun built-in networking)")
	// TSI: guest port is directly accessible on host via libkrun
	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	vmc.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.LocalHost, strconv.FormatUint(port, 10))
	vmc.SSHInfo.HostSSHProxyListenAddr = vmc.SSHInfo.GuestSSHServerListenAddr
	return nil
}
