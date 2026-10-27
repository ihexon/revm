//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/commanderr"
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
		return runVM(ctx, cfg)
	default:
		return fmt.Errorf("unsupported run mode %q", cfg.RunMode)
	}
}
