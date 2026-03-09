package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/network"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
)

const (
	eth0     = "eth0"
	attempts = 3
)

const (
	resolveFile       = "/etc/resolv.conf"
	defaultNameServer = "nameserver 1.1.1.1"

	// Write the podman-machine marker so podman inside the VM calls gvproxy's
	// expose/unexpose API on container start/stop, enabling -p port forwarding.
	// "applehv" matches the constant used by podman for Apple Hypervisor based VMs.
	//
	// See:
	// https://github.com/containers/podman/blob/a98154a9782670b3398719d48565cc2510fd0152/libpod/networking_machine.go
	machineMarker = "/etc/containers/podman-machine"
)

// ConfigureNetwork must support TSI/Gvisor network
func ConfigureNetwork(ctx context.Context, mode define.VNetMode) error {
	if mode == define.TSI {
		_ = os.Remove(machineMarker)
		return os.WriteFile(resolveFile, []byte(defaultNameServer), 0644)
	}

	if mode == define.GVISOR {
		if err := os.MkdirAll(filepath.Dir(machineMarker), 0755); err != nil {
			return fmt.Errorf("create podman-machine marker dir: %w", err)
		}

		if err := os.WriteFile(machineMarker, []byte("applehv"), 0644); err != nil {
			return fmt.Errorf("write podman-machine marker: %w", err)
		}

		return network.DHClient4(ctx, eth0, attempts)
	}

	return fmt.Errorf("unsupported network mode: %s", mode)
}
