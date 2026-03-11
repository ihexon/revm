package interfaces

import (
	"context"
	"linuxvm/pkg/define"
)

type VMMProvider interface {
	Start(ctx context.Context) error
	Stop() error
	GetVMConfig() *define.Machine
}
