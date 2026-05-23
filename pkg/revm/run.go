//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}

	switch cfg.RunMode {
	case ModeAttach:
		return Attach(ctx, cfg)
	case ModeControl:
		return Control(ctx, cfg)
	case ModeRootfs, ModeContainer:
		vm, err := Build(ctx, cfg)
		if err != nil {
			return err
		}
		defer vm.Release()
		return vm.Run(ctx)
	default:
		return fmt.Errorf("unsupported run mode %q", cfg.RunMode)
	}
}
