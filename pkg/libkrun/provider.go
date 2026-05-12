//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

import (
	"context"
	"linuxvm/pkg/define"
	"runtime"
)

type Provider struct {
	mc      *define.Machine
	libkrun *Libkrun
}

func NewProvider(mc *define.Machine) *Provider {
	return &Provider{mc: mc, libkrun: New(mc)}
}

func (p *Provider) Create(ctx context.Context) error {
	return p.libkrun.Create(ctx)
}

func (p *Provider) Start(ctx context.Context) error {
	ch := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		ch <- p.libkrun.Start(ctx)
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		_ = p.Stop()
		return ctx.Err()
	}
}

func (p *Provider) Stop() error {
	p.libkrun.SendSignal(define.GuestSignalTerminated)
	return nil
}
