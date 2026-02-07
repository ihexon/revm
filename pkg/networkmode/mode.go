//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package networkmode

import (
	"context"
	"linuxvm/pkg/define"
)

// Mode represents a virtual network mode with its specific capabilities and behaviors.
// This interface encapsulates all network mode differences including service lifecycle.
type Mode interface {
	// Service lifecycle methods

	// StartNetworkStack starts the network stack required by this mode.
	// For GVISOR: starts gvisor-tap-vsock
	// For TSI: no-op (uses built-in networking)
	StartNetworkStack(ctx context.Context, vmc *define.VMConfig) error

	// WaitNetworkReady waits for the network stack to be ready.
	// For GVISOR: waits for GVProxy probe
	// For TSI: no-op
	WaitNetworkReady(ctx context.Context, vmc *define.VMConfig) error

	// StartPodmanProxy starts the Podman API proxy if needed (docker mode only).
	// For GVISOR: starts Unix socket proxy
	// For TSI: no-op (uses direct TCP)
	StartPodmanProxy(ctx context.Context, vmc *define.VMConfig) error

	// WaitPodmanReady waits for Podman API to be accessible.
	// For GVISOR: waits for Unix socket proxy
	// For TSI: waits for TCP connection
	WaitPodmanReady(ctx context.Context, vmc *define.VMConfig) error

	// Address getters

	// GetPodmanListenAddr returns the address where Podman API should be accessed.
	// For GVISOR: unix socket path
	// For TSI: TCP address (localhost:port)
	GetPodmanListenAddr(vmc *define.VMConfig) string

	// String returns the mode name (for logging and serialization).
	String() string
}

// FromString creates a Mode instance from a string representation.
// Returns nil if the mode string is invalid.
func FromString(modeStr string) Mode {
	switch modeStr {
	case define.GVISOR.String():
		return &GVisorMode{}
	case define.TSI.String():
		return &TSIMode{}
	default:
		return nil
	}
}

// FromVNetMode creates a Mode instance from VNetMode enum.
func FromVNetMode(mode define.VNetMode) Mode {
	return FromString(mode.String())
}
