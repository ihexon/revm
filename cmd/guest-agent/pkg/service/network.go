package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/network"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
)

// The marker makes Podman publish container ports through gvproxy.
const machineMarker = "/etc/containers/podman-machine"

func ConfigureNetwork(ctx context.Context, mode, resolvConf string) error {
	switch mode {
	case define.NetworkTSI:
		// Use the host resolver snapshot without configuring a virtual interface.
		// No virtual interface or gvproxy port publishing exists in this mode.
		if err := os.Remove(machineMarker); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile("/etc/resolv.conf", []byte(resolvConf), 0644)
	case define.NetworkGVisor:
	default:
		return fmt.Errorf("unsupported network mode %q", mode)
	}
	if err := os.MkdirAll(filepath.Dir(machineMarker), 0755); err != nil {
		return fmt.Errorf("create podman-machine marker dir: %w", err)
	}
	if err := os.WriteFile(machineMarker, []byte("applehv"), 0644); err != nil {
		return fmt.Errorf("write podman-machine marker: %w", err)
	}
	// Use gvproxy's fixed address plan without depending on DHCP broadcasts.
	return network.ConfigureStaticIPv4(ctx, "eth0", define.GuestCIDR, define.GatewayIP, define.GatewayIP)
}
