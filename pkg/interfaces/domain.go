package interfaces

import (
	"context"
	"linuxvm/pkg/define"
)

type VMMProvider interface {
	Create(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetVMConfigure() *define.Machine
	StartVMCtlServer(ctx context.Context, stopFn func()) error
}
