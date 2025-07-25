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
}
