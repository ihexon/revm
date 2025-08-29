//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/vmconfig"
)

func Get(vmc *vmconfig.VMConfig) Provider {
	return libkrun.NewAppleHyperVisor(vmc)
}
