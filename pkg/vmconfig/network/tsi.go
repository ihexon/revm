//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig/internal"

	"github.com/sirupsen/logrus"
)

// TSINetworkConfig implements network configuration for TSI (Transparent Socket Interception) mode.
// TSI mode uses libkrun's built-in network capabilities without external network stack.
type TSINetworkConfig struct{}

// Configure sets up TSI network mode.
// TSI mode doesn't require gvisor network setup, so this is essentially a no-op.
func (t *TSINetworkConfig) Configure(ctx context.Context, vmc *define.VMConfig, pathMgr *internal.PathManager) error {
	logrus.Infof("Using TSI network mode (libkrun built-in networking)")
	// TSI mode doesn't need gvisor network setup
	return nil
}
