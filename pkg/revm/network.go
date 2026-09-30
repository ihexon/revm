//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sirupsen/logrus"
)

// configureNetwork records the selected guest network and allocates the host
// endpoints needed by that mode. There are only two supported modes, so a
// switch is clearer than a strategy registry.
func (v *machineBuilder) configureNetwork(mode define.VNetMode) error {
	v.VirtualNetworkMode = mode
	switch mode {
	case define.GVISOR:
		return v.configureGVisorNetwork()
	case define.TSI:
		return v.configureTSINetwork()
	default:
		return fmt.Errorf("invalid network mode: %s", mode)
	}
}

func (v *machineBuilder) configureGVisorNetwork() error {
	logrus.Infof("Configuring gvisor-tap-vsock network mode")
	pathMgr := v.pathMgr
	controlPath := pathMgr.GetGVPCtlSocketFile()
	v.GVPCtlAddr = (&url.URL{Scheme: "unix", Path: controlPath}).String()
	v.GVPVNetAddr = fmt.Sprintf("unixgram://%s", pathMgr.GetVNetSocketFile())
	v.GVPNotifyAddr = fmt.Sprintf("unix://%s", pathMgr.GetGVPNotifySocketFile())

	for _, path := range []string{controlPath, pathMgr.GetVNetSocketFile(), pathMgr.GetGVPNotifySocketFile()} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale network socket %q: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(controlPath), 0755); err != nil {
		return err
	}

	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	v.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.UnspecifiedAddress, strconv.FormatUint(port, 10))
	forwardPort, err := network.GetAvailablePort(define.SSHLocalForwardListenPort)
	if err != nil {
		return fmt.Errorf("get available port for ssh forwarding: %w", err)
	}
	v.SSHInfo.HostSSHProxyListenAddr = net.JoinHostPort(define.LocalHost, strconv.FormatUint(forwardPort, 10))
	return nil
}

func (v *machineBuilder) configureTSINetwork() error {
	logrus.Infof("Using TSI network mode (libkrun built-in networking)")
	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}
	v.SSHInfo.GuestSSHServerListenAddr = net.JoinHostPort(define.LocalHost, strconv.FormatUint(port, 10))
	v.SSHInfo.HostSSHProxyListenAddr = v.SSHInfo.GuestSSHServerListenAddr
	return nil
}
