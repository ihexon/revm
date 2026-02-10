package interfaces

import (
	"context"
	"linuxvm/pkg/vmbuilder"
)

type VMMProvider interface {
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() (*vmbuilder.VMConfig, error)
	StartVMCtlServer(ctx context.Context) error
}
