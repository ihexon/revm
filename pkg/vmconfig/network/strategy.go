//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig/internal"
)

// NetworkConfigStrategy defines the interface for network configuration strategies.
// Different network modes (GVISOR, TSI) implement this interface to configure
// the VM's network stack in their specific way.
type NetworkConfigStrategy interface {
	// Configure sets up network configuration on the given VMConfig.
	// pathMgr is used to get workspace-relative paths for socket files.
	Configure(ctx context.Context, vmc *define.VMConfig, pathMgr *internal.PathManager) error
}
