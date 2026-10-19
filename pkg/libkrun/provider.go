//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

import (
	"context"
	"errors"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/guestcontrol"
	"runtime"
	"sync"
)

type Provider struct {
	mc      *define.MachineSpec
	libkrun *Libkrun
	mu      sync.Mutex
	state   providerState
}

type providerState uint8

const (
	providerCreated providerState = iota
	providerRunning
	providerExited
	providerClosed
)

func NewProvider(ctx context.Context, mc *define.MachineSpec) (*Provider, error) {
	p := &Provider{mc: mc, libkrun: New(mc), state: providerCreated}
	if err := p.libkrun.Create(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) Start(_ context.Context) error {
	p.mu.Lock()
	if p.state != providerCreated {
		p.mu.Unlock()
		return errors.New("libkrun: VMM can only be started once")
	}
	p.state = providerRunning
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.state = providerExited
		p.mu.Unlock()
	}()

	// krun_vmm_run is a consuming, blocking call. It must stay on its locked
	// OS thread and must be allowed to return before any handle is destroyed.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return p.libkrun.Start(context.Background())
}

func (p *Provider) RequestShutdown(ctx context.Context) error {
	controlTarget := guestcontrol.DefaultTarget()
	if addr, err := network.ParseUnixAddr(p.mc.GuestControlAddr); err == nil {
		controlTarget.UnixSocket = addr.Path
	}
	return guestcontrol.Shutdown(ctx, controlTarget)
}

func (p *Provider) Close() error {
	p.mu.Lock()
	if p.state != providerRunning {
		p.state = providerClosed
	}
	running := p.state == providerRunning
	p.mu.Unlock()
	if running {
		return errors.New("libkrun: cannot close while VMM is running")
	}
	return p.libkrun.Close()
}
