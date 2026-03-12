//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

import (
	"context"
	"linuxvm/pkg/define"
	"runtime"
)

type Provider struct {
	mc *define.Machine
	vm *VM
}

func NewProvider(mc *define.Machine) *Provider {
	return &Provider{mc: mc, vm: New(mc)}
}

func (p *Provider) Create(ctx context.Context) error {
	return p.vm.Create(ctx)
}

func (p *Provider) Start(ctx context.Context) error {
	ch := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		ch <- p.vm.Start(ctx)
	}()
	return <-ch
}

func (p *Provider) Stop() error {
	p.vm.SendSignal("terminated")
	return nil
}

func (p *Provider) GetVMConfig() *define.Machine {
	return p.mc
}
