//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"linuxvm/pkg/define"
)

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
