//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/execution"
)

func Run(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}

	normalized, err := NormalizeConfig(*cfg)
	if err != nil {
		return commanderr.Usage(err)
	}
	cfg = &normalized
	switch cfg.RunMode {
	case ModeExec:
		return execSession(ctx, cfg)
	case ModeControl:
		return controlSession(ctx, cfg)
	case ModeRootfs, ModeContainer:
		vm, err := buildVM(ctx, cfg)
		if err != nil {
			return err
		}
		runErr := vm.Run(ctx)
		if controller := execution.ControllerFrom(ctx); controller != nil {
			if cause := controller.Cause(); cause != nil {
				runErr = errors.Join(cause, runErr)
			}
		}
		recordErr := vm.lease.finish(vm.result, runErr)
		releaseErr := vm.Release()
		return errors.Join(runErr, recordErr, releaseErr)
	default:
		return fmt.Errorf("unsupported run mode %q", cfg.RunMode)
	}
}
