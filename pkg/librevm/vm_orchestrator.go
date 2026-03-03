//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/service/lifecycle"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type hostServices interface {
	StartPodmanProxy(ctx context.Context) error
	StartNetworkStack(ctx context.Context) error
	StartIgnitionService(ctx context.Context) error
	StartMachineManagementAPI(ctx context.Context, stopFn func()) error
	StartVirtualMachine(ctx context.Context) error
	ExitVirtualMachineWhenSomethingHappened(ctx context.Context) error
}

// New creates a VM from the given Config. It resolves defaults, validates the
// configuration, sets up the workspace and file lock, builds the internal
// Machine, and prepares the VM provider.
//
// Close must be called even if Start is never called.
func New(ctx context.Context, cfg *Config, opts ...OptionFn) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	var o vmOptions
	for _, fn := range opts {
		fn(&o)
	}
	success := false
	defer func() {
		if success {
			return
		}
		o.dispatcher.close()
	}()

	normalized, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}
	if err := validateConfig(normalized); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	workspacePath := workspacePathForSession(normalized.Name)
	mc, fl, err := buildMachine(ctx, normalized, workspacePath)
	if err != nil {
		return nil, fmt.Errorf("build machine: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		return nil, fmt.Errorf("create vm provider: %w", err)
	}

	vm := &VM{
		cfg:           &normalized,
		opts:          &o,
		machine:       mc,
		provider:      vmp,
		svc:           lifecycle.NewHostServices(vmp),
		workspacePath: workspacePath,
		fileLock:      fl,
		state:         vmStateNew,
		stopper:       newStopController(mc),
	}
	vm.emit(EventConfiguring, "resolving defaults and validating config")
	success = true
	return vm, nil
}

// Run launches all host services, runs the VM to completion, drains the
// services, and returns the VM's exit error. It blocks for the lifetime
// of the VM.
func (vm *VM) Run(ctx context.Context) error {
	vm.mu.Lock()
	if vm.state == vmStateClosed {
		vm.mu.Unlock()
		return fmt.Errorf("vm is closed")
	}
	if vm.state != vmStateNew {
		vm.mu.Unlock()
		return fmt.Errorf("vm already started")
	}
	vm.state = vmStateRunning
	vm.mu.Unlock()
	defer func() {
		vm.mu.Lock()
		if vm.state == vmStateRunning || vm.state == vmStateStopping {
			vm.state = vmStateStopped
		}
		vm.mu.Unlock()
	}()

	vm.emit(EventVMStarting, "starting vm")

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return vm.svc.ExitVirtualMachineWhenSomethingHappened(gctx) })
	g.Go(func() error {
		vm.emit(EventIgnitionStarting, "starting ignition service")
		return vm.svc.StartIgnitionService(gctx)
	})
	g.Go(func() error {
		vm.emit(EventNetworkStarting, "starting network stack")
		return vm.svc.StartNetworkStack(gctx)
	})
	g.Go(func() error {
		vm.emit(EventManagementStarting, "starting management API")
		return vm.svc.StartMachineManagementAPI(gctx, func() { _ = vm.Stop(context.Background()) })
	})

	switch vm.cfg.Mode {
	case ModeContainer:
		g.Go(func() error {
			go func() {
				select {
				case <-gctx.Done():
					return
				case <-vm.machine.Readiness.PodmanReady:
					vm.emit(EventPodmanReady, fmt.Sprintf("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr))
					logrus.Infof("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
				}
			}()
			select {
			case <-gctx.Done():
				return gctx.Err()
			case <-vm.machine.Readiness.VNetHostReady:
				vm.emit(EventPodmanProxyStarting, "starting podman proxy")
				return vm.svc.StartPodmanProxy(gctx)
			}
		})
	case ModeRootfs:
	}

	g.Go(func() error {
		select {
		case <-gctx.Done():
			return gctx.Err()
		case <-vm.machine.Readiness.SSHReady:
			vm.emit(EventSSHReady, "ssh ready")
		}
		return nil
	})

	select {
	case <-gctx.Done():
		g.Wait() //nolint:errcheck
		return context.Cause(gctx)
	case <-vm.machine.Readiness.VNetHostReady:
		vm.emit(EventNetworkReady, "host network ready")
	}

	vmErr := vm.svc.StartVirtualMachine(ctx)
	vm.requestStop()
	g.Wait() //nolint:errcheck
	if vmErr != nil {
		vm.emit(EventError, vmErr.Error())
	}
	vm.emit(EventStopped, "vm stopped")
	return vmErr
}

func (vm *VM) emit(kind EventKind, msg string) {
	if vm == nil || vm.opts == nil {
		return
	}
	vm.opts.dispatcher.publish(kind, msg, vm.cfg.Name, vm.seq.Add(1))
}
