//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"context"
	"linuxvm/pkg/define"
)

// HostService represents a virtual network mode with its specific capabilities and behaviors.
// This interface encapsulates all network mode differences including service lifecycle.
type HostService interface {
	// StartNetworkStack starts the network stack required by this mode.
	// For GVISOR: starts gvisor-tap-vsock
	// For TSI: no-op (uses built-in networking)
	StartNetworkStack(ctx context.Context, vmc *define.VMConfig) error

	// StartPodmanProxy starts the Podman API proxy if needed (docker mode only).
	// For GVISOR: starts Unix socket proxy (caller must ensure gvproxy is ready)
	// For TSI: no-op (uses direct TCP)
	StartPodmanProxy(ctx context.Context, vmc *define.VMConfig) error

	// GetPodmanListenAddr returns the address where Podman API should be accessed.
	// For GVISOR: unix socket path
	// For TSI: TCP address (localhost:port)
	GetPodmanListenAddr(vmc *define.VMConfig) string

	// String returns the mode name (for logging and serialization).
	String() string
}

func NewHostServiceManager(mode define.VNetMode) HostService {
	switch mode {
	case define.GVISOR:
		return &GVisorMode{}
	case define.TSI:
		return &TSIMode{}
	default:
		return nil
	}
}
