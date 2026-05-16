//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

import (
	"context"
	"linuxvm/pkg/define"
	"runtime"
)

type Provider struct {
	mc      *define.MachineSpec
	libkrun *Libkrun
}

func NewProvider(mc *define.MachineSpec) *Provider {
	return &Provider{mc: mc, libkrun: New(mc)}
}

func (p *Provider) Create(ctx context.Context) error {
	return p.libkrun.Create(ctx)
}

func (p *Provider) Start(vmWaitAbortCtx context.Context) error {
	ch := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// vmWaitAbortCtx follows the blocking libkrun Start call. Graceful shutdown is
		// requested separately through RequestShutdown, not by cancelling this ctx.
		ch <- p.libkrun.Start(vmWaitAbortCtx)
	}()

	select {
	case err := <-ch:
		return err
	case <-vmWaitAbortCtx.Done():
		return vmWaitAbortCtx.Err()
	}
}

func (p *Provider) RequestShutdown(ctx context.Context) error {
	return p.libkrun.SendSignal(ctx, define.GuestSignalTerminated)
}

func (p *Provider) ForceStop(ctx context.Context) error {
	return p.libkrun.SendSignal(ctx, define.GuestSignalTerminated)
}
