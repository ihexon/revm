//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type Provider interface {
	StartNetwork(ctx context.Context) error
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() (*vmconfig.VMConfig, error)
}