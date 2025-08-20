//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type Provider interface {
	StartNetwork(ctx context.Context, vmc *vmconfig.VMConfig) error
	Create(ctx context.Context, vmc *vmconfig.VMConfig, cmdline *vmconfig.Cmdline) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	SyncTime(ctx context.Context) error
}
