//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vm

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type Provider interface {
	StartNetwork(ctx context.Context) error
	Create(ctx context.Context, cmdline *vmconfig.Cmdline) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsSSHReady(ctx context.Context) bool
	SyncTime(ctx context.Context) error
}
