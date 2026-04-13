//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/service/lifecycle"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// VM represents a running (or ready-to-run) virtual machine.
// Close must always be called to release resources.
type VM struct {
	cfg *Config

	machine         *define.Machine
	provider        interfaces.VMMProvider
	svc             lifecycle.HostServices
	sessionDir      string
	cleanup         func()
	eventDispatcher eventDispatcher
	Cancel          context.CancelFunc

	seq atomic.Uint64
}

// newProvider creates a libkrun Provider for the current platform.
func newProvider(mc *define.Machine) (interfaces.VMMProvider, error) {
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
	case runtime.GOOS == "linux" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	p := libkrun.NewProvider(mc)
	if err := p.Create(context.Background()); err != nil {
		return nil, fmt.Errorf("create libkrun libkrun: %w", err)
	}
	return p, nil
}

// Close 释放运行时资源（文件锁、event eventDispatcher）。
// 必须始终调用，即使 Run() 从未被调用。幂等。
func (vm *VM) Close() error {
	if vm.cleanup != nil {
		vm.cleanup()
	}
	vm.eventDispatcher.close()
	return nil
}

func New(cfg *Config) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}

	vm := &VM{
		cfg:        &normalizedCfg,
		sessionDir: getSessionDir(normalizedCfg.SessionID),
	}

	if reporter := newEventReporter(normalizedCfg.ReportURL); reporter != nil {
		vm.eventDispatcher.addReporter(reporter)
	}

	return vm, nil
}

// init acquires all heavyweight resources: workspace dirs, flock, SSH keys,
// disk images, libkrun provider, and host services. Called once at the
// start of Run(). On failure it cleans up after itself.
func (vm *VM) init(ctx context.Context) error {
	mc, cleanup, err := buildMachine(ctx, *vm.cfg, vm.sessionDir)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}

	if err := vm.createUserSymlinks(); err != nil {
		cleanup()
		return fmt.Errorf("create symlinks: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		cleanup()
		return fmt.Errorf("create vm provider: %w", err)
	}

	vm.machine = mc
	vm.provider = vmp
	vm.svc = lifecycle.NewHostServices(vmp)
	vm.cleanup = cleanup
	return nil
}

// createUserSymlinks links session-internal resources to user-specified paths.
// All actual files remain inside sessionDir; the symlinks are just a convenience
// bridge so external tools can find them at well-known locations.
func (vm *VM) createUserSymlinks() error {
	cfg := vm.cfg
	p := newMachinePathManager(vm.sessionDir)

	if cfg.PodmanProxyAPIFile != "" {
		if err := createSymlink(p.GetPodmanSocketFile(), cfg.PodmanProxyAPIFile); err != nil {
			return fmt.Errorf("podman proxy socket: %w", err)
		}
	}
	if cfg.ManageAPIFile != "" {
		if err := createSymlink(p.GetVMCtlSocketFile(), cfg.ManageAPIFile); err != nil {
			return fmt.Errorf("vmctl socket: %w", err)
		}
	}

	if cfg.SSHKeyFileSymbolPath != "" {
		// link ssh private key to user-specified path
		sshKeyPath := p.GetSSHKeyFilePath()
		if err := createSymlink(sshKeyPath, cfg.SSHKeyFileSymbolPath); err != nil {
			return fmt.Errorf("ssh private key: %w", err)
		}

		// link ssh public key to user-specified path
		if err := createSymlink(sshKeyPath+".pub", cfg.SSHKeyFileSymbolPath+".pub"); err != nil {
			return fmt.Errorf("ssh public key: %w", err)
		}
	}

	return nil
}

// RunChroot starts the VM in rootfs mode and blocks until it exits.
func (vm *VM) RunChroot(ctx context.Context) error {
	if err := vm.init(ctx); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	// Start ignition server
	g.Go(func() error {
		vm.emit(EventIgnitionService, "starting ignition service")
		return vm.svc.StartIgnitionService(ctx)
	})

	// Start network stack
	g.Go(func() error {
		vm.emit(EventHostNetworkStack, "starting host network stack")
		return vm.svc.StartHostNetworkStack(ctx)
	})

	// Start management API
	g.Go(func() error {
		vm.emit(EventManagementAPIStarting, "starting vm management api")
		return vm.svc.StartMachineManagementAPI(ctx)
	})

	// Monitor readiness events
	go vm.monitorReadinessEvents(ctx, false)

	// Monitor for shutdown signals
	go func() {
		vm.WaitAndShutdownMachine(ctx, vm.Cancel)
	}()

	// Wait for services to start
	svcErrCh := make(chan error, 1)
	go func() {
		svcErrCh <- g.Wait()
		close(svcErrCh)
	}()

	// Start libkrun when network is ready
	select {
	case <-ctx.Done():
		return <-svcErrCh
	case <-vm.machine.Readiness.VNetHostReady: // Start libkrun when network is ready
		reason := fmt.Errorf("boot virtual machine")
		logrus.Info(reason.Error())
		vm.emit(EventVirtualMachineBooting, reason.Error())
		err := vm.svc.StartVirtualMachine(ctx)
		vm.Cancel()
		<-svcErrCh
		return err
	}
}

// RunDocker starts the VM in container mode and blocks until it exits.
func (vm *VM) RunDocker(ctx context.Context) error {
	if err := vm.init(ctx); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	// Start ignition server
	g.Go(func() error {
		vm.emit(EventIgnitionService, "starting ignition service")
		return vm.svc.StartIgnitionService(ctx)
	})

	// Start network stack
	g.Go(func() error {
		vm.emit(EventHostNetworkStack, "starting host network stack")
		return vm.svc.StartHostNetworkStack(ctx)
	})

	// Start management API
	g.Go(func() error {
		vm.emit(EventManagementAPIStarting, "starting vm management api")
		return vm.svc.StartMachineManagementAPI(ctx)
	})

	// Start Podman proxy (wait for network first)
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vm.machine.Readiness.VNetHostReady:
			return vm.svc.StartPodmanProxy(ctx)
		}
	})

	// Monitor readiness events
	go vm.monitorReadinessEvents(ctx, true)

	// Monitor for shutdown signals
	go func() {
		vm.WaitAndShutdownMachine(ctx, vm.Cancel)
	}()

	// Wait for services to start
	svcErrCh := make(chan error, 1)
	go func() {
		svcErrCh <- g.Wait()
		close(svcErrCh)
	}()

	select {
	case <-ctx.Done():
		return <-svcErrCh
	case <-vm.machine.Readiness.VNetHostReady: // Start libkrun when network is ready
		reason := fmt.Errorf("boot virtual machine")
		logrus.Info(reason.Error())
		vm.emit(EventVirtualMachineBooting, reason.Error())
		err := vm.svc.StartVirtualMachine(ctx)
		vm.Cancel()
		<-svcErrCh
		return err
	}
}

// monitorReadinessEvents monitors readiness channels and emits events.
// It runs until all expected events are received or context is cancelled.
func (vm *VM) monitorReadinessEvents(ctx context.Context, expectPodman bool) {
	podmanReady := !expectPodman // If not expecting, mark as already done
	sshReady := false
	networkReady := false

	for {
		if podmanReady && sshReady && networkReady {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-vm.machine.Readiness.PodmanReady:
			if expectPodman && !podmanReady {
				podmanReady = true
				vm.emit(EventPodmanReady, fmt.Sprintf("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr))
				logrus.Infof("podman API proxy ready on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
			}
		case <-vm.machine.Readiness.SSHReady:
			if !sshReady {
				sshReady = true
				vm.emit(EventSSHReady, "ssh ready")
			}
		case <-vm.machine.Readiness.VNetHostReady:
			if !networkReady {
				networkReady = true
				vm.emit(EventNetworkReady, "host network ready")
			}
		}
	}
}

func (vm *VM) WaitAndShutdownMachine(ctx context.Context, cancel context.CancelFunc) {
	// Monitor parent process exit
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() == 1 {
					reason := fmt.Errorf("parent process exited, shutting down machine")
					logrus.Info(reason.Error())
					vm.emit(EventStopping, reason.Error())
					_ = vm.svc.StopVirtualMachine()
					cancel()
					return
				}
			}
		}
	}()

	// Monitor shutdown signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			reason := fmt.Errorf("received signal, shutting down")
			logrus.Info(reason.Error())
			vm.emit(EventStopping, reason.Error())
			_ = vm.svc.StopVirtualMachine()
			cancel()
		}
	}()
}

// emit sending events with option msg
func (vm *VM) emit(kind EventKind, msg string) {
	if vm == nil || vm.cfg == nil {
		return
	}
	vm.eventDispatcher.emit(vm.cfg.SessionID, vm.cfg.RunMode, kind, msg, vm.seq.Add(1))
}
