package interfaces

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type VMMProvider interface {
	StartNetwork(ctx context.Context) error
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() (*vmconfig.VMConfig, error)
	StartIgnServer(ctx context.Context) error
	StartVMCtlServer(ctx context.Context) error
}
