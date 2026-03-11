//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

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

// networkConfigStrategy defines the interface for network configuration strategies.
// Different network modes (GVISOR, TSI) implement this interface to configure
// the VM's network stack in their specific way.
type networkConfigStrategy interface {
	// Configure sets up network configuration on the given VM.
	Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error
}

// getNetworkStrategy returns the appropriate network strategy for the given network mode.
// Returns nil if the mode is invalid/unknown.
func getNetworkStrategy(mode define.VNetMode) networkConfigStrategy {
	switch mode {
	case define.GVISOR:
		return &gVisorNetworkConfig{}
	case define.TSI:
		return &tsiNetworkConfig{}
	default:
		return nil
	}
}

// gVisorNetworkConfig implements network configuration for gvisor-tap-vsock mode.
// This mode uses gvisor's userspace network stack with vsock communication.
type gVisorNetworkConfig struct{}

// Configure sets up the gvisor-tap-vsock network configuration.
// It creates Unix socket paths for GVProxy control and virtual network communication.
func (g *gVisorNetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error {
	logrus.Infof("Configuring gvisor-tap-vsock network mode")

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetGVPCtlSocketFile(),
	}

	vmc.GVPCtlAddr = unixAddr.String()

	// On Linux, use unix:// (stream socket for QemuProtocol).
	// On macOS, use unixgram:// (datagram socket for VfkitProtocol).
	if runtime.GOOS == "linux" {
		vmc.GVPVNetAddr = fmt.Sprintf("unix://%s", pathMgr.GetVNetSocketFile())
	} else {
		vmc.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetSocketFile())
	}

	// Clean up any existing sockets
	_ = os.Remove(pathMgr.GetGVPCtlSocketFile())
	_ = os.Remove(pathMgr.GetVNetSocketFile())

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

// tsiNetworkConfig implements network configuration for TSI (Transparent Socket Interception) mode.
// TSI mode uses libkrun's built-in network capabilities without external network stack.
type tsiNetworkConfig struct{}

// Configure sets up TSI network mode.
// TSI mode doesn't require gvisor network setup, but we record the host-accessible
// SSH address since guest ports are directly reachable via libkrun.
func (t *tsiNetworkConfig) Configure(ctx context.Context, vmc *define.Machine, pathMgr *machinePathManager) error {
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

func (v *machineBuilder) configureNetwork(ctx context.Context, mode define.VNetMode) error {
	strategy := getNetworkStrategy(mode)
	if strategy == nil {
		return fmt.Errorf("invalid network mode: %s", mode)
	}
	v.VirtualNetworkMode = mode
	return strategy.Configure(ctx, &v.Machine, v.pathMgr)
}
