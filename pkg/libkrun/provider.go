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
	"time"

	"github.com/sirupsen/logrus"
)

type Provider struct {
	mc                   *define.MachineSpec
	libkrun              *Libkrun
	mu                   sync.Mutex
	running              bool
	started              bool
	runDone              chan struct{}
	state                providerState
	shutdownFallbackOnce sync.Once
}

type providerState uint8

const (
	providerCreated providerState = iota
	providerRunning
	providerExited
	providerClosed
)

func NewProvider(ctx context.Context, mc *define.MachineSpec) (*Provider, error) {
	p := &Provider{mc: mc, libkrun: New(mc), runDone: make(chan struct{}), state: providerCreated}
	if err := p.libkrun.Create(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) Start(_ context.Context) error {
	p.mu.Lock()
	if p.started || p.state == providerClosed {
		p.mu.Unlock()
		return errors.New("libkrun: VMM can only be started once")
	}
	p.started = true
	p.running = true
	p.state = providerRunning
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.running = false
		p.state = providerExited
		close(p.runDone)
		p.mu.Unlock()
	}()

	// krun_vmm_run is a consuming, blocking call. It must stay on its locked
	// OS thread and must be allowed to return before any handle is destroyed.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return p.libkrun.Start(context.Background())
}

func (p *Provider) RequestShutdown(ctx context.Context) error {
	nativeErr := p.libkrun.Shutdown(ctx)
	if nativeErr == nil {
		// libkrun owns the VMM lifecycle. Once the native shutdown event has been
		// accepted, give the guest a short opportunity to handle it. Alpine's
		// minimal init may not consume the GPIO/restart-key event, so retain a
		// delayed GuestControl fallback without racing a successful shutdown.
		p.scheduleGuestShutdownFallback()
		return nil
	}
	// Older libkrun builds and guests without shutdown support can still use the
	// guest-control endpoint as a compatibility fallback.
	guestErr := p.requestGuestShutdown(ctx)
	if guestErr == nil {
		return nil
	}
	if err := p.libkrun.SendSignal(ctx, define.GuestSignalTerminated); err != nil {
		return errors.Join(nativeErr, guestErr, err)
	}
	return nil
}

func (p *Provider) requestGuestShutdown(ctx context.Context) error {
	controlTarget := guestcontrol.DefaultTarget()
	if addr, err := network.ParseUnixAddr(p.mc.GuestControlAddr); err == nil {
		controlTarget.UnixSocket = addr.Path
	}
	return guestcontrol.Shutdown(ctx, controlTarget)
}

func (p *Provider) scheduleGuestShutdownFallback() {
	p.shutdownFallbackOnce.Do(func() {
		go func() {
			timer := time.NewTimer(750 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-p.runDone:
				return
			case <-timer.C:
			}
			logrus.Debug("native libkrun shutdown did not finish guest exit; using GuestControl compatibility fallback")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := p.requestGuestShutdown(ctx); err != nil {
				// Native shutdown remains authoritative; this is only the
				// compatibility path for guests that do not handle the event.
				return
			}
		}()
	})
}

func (p *Provider) Pause(ctx context.Context) error {
	return p.libkrun.Pause(ctx)
}

func (p *Provider) Resume(ctx context.Context) error {
	return p.libkrun.Resume(ctx)
}

func (p *Provider) ForceStop(ctx context.Context) error {
	if err := p.RequestShutdown(ctx); err != nil {
		return err
	}
	select {
	case <-p.runDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Provider) Close() error {
	p.mu.Lock()
	running := p.running
	if !running {
		p.state = providerClosed
	}
	p.mu.Unlock()
	if running {
		return errors.New("libkrun: cannot close while VMM is running")
	}
	return p.libkrun.Close()
}
