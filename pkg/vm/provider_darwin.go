//go:build darwin && arm64

package vm

import "linuxvm/pkg/libkrun"

func Get() Provider {
	return libkrun.NewAppleHyperVisor()
}
