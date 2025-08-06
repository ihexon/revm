//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import "linuxvm/pkg/libkrun"

func Get() Provider {
	return libkrun.NewAppleHyperVisor()
}
