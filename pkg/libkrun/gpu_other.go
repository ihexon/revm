//go:build !darwin || !arm64

package libkrun

import (
	"fmt"
	"linuxvm/pkg/define"
)

func (v *Libkrun) setupGPU() error {
	if v.cfg.GPUBackend == "" || v.cfg.GPUBackend == define.GPUOff {
		return nil
	}
	return fmt.Errorf("libkrun: GPU backend %q is unavailable on this platform", v.cfg.GPUBackend)
}
