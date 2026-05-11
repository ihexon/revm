//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/ignition"
	"linuxvm/pkg/service/management"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
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

	machine          *define.Machine
	provider         interfaces.VMMProvider
	sessionDir       string
	releaseResources func()
	eventDispatcher  eventDispatcher

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
	if err := libkrun.CheckHostSupport(); err != nil {
		return nil, err
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
	if vm.releaseResources != nil {
		vm.releaseResources()
		vm.releaseResources = nil
	}
	vm.eventDispatcher.close()
	return nil
}

func buildTimeInfo() string {
	version := define.Version
	if version == "" {
		version = "unknown"
	}
	commit := define.CommitID
	if commit == "" {
		commit = "unknown"
	}
	buildDate := define.BuildDate
	if buildDate == "" {
		buildDate = "unknown"
	}

	return fmt.Sprintf("%s-%s-%s", version, commit, buildDate)
}

func New(cfg *Config) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	logrus.Infof("revm build info: %s", buildTimeInfo())

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
// disk images, and libkrun provider. Called once at the start of Run().
// On failure it cleans up after itself.
func (vm *VM) init(ctx context.Context) error {
	mc, releaseResources, err := buildMachine(ctx, *vm.cfg, vm.sessionDir)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}

	if err := vm.createUserSymlinks(); err != nil {
		releaseResources()
		return fmt.Errorf("create symlinks: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		releaseResources()
		return fmt.Errorf("create vm provider: %w", err)
	}

	vm.machine = mc
	vm.provider = vmp
	vm.releaseResources = releaseResources
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

	ctx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(context.Canceled)

	networkReady, signalNetworkReady := vm.newNetworkReadySignal()

	var g errgroup.Group

	vm.startRunService(&g, ctx, cancelRun, vm.startIgnitionService)
	vm.startRunService(&g, ctx, cancelRun, func(ctx context.Context) error {
		return vm.startHostNetworkStack(ctx, signalNetworkReady)
	})
	vm.startRunService(&g, ctx, cancelRun, vm.startMachineManagementAPI)

	go func() {
		vm.WaitAndShutdownMachine(ctx, func() {
			cancelRun(context.Canceled)
		})
	}()

	g.Go(func() error {
		return vm.runVirtualMachine(ctx, cancelRun, networkReady)
	})

	return runError(ctx, g.Wait())
}

// RunDocker starts the VM in container mode and blocks until it exits.
func (vm *VM) RunDocker(ctx context.Context) error {
	if err := vm.init(ctx); err != nil {
		return err
	}

	ctx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(context.Canceled)

	networkReady, signalNetworkReady := vm.newNetworkReadySignal()

	var g errgroup.Group

	vm.startRunService(&g, ctx, cancelRun, vm.startIgnitionService)
	vm.startRunService(&g, ctx, cancelRun, func(ctx context.Context) error {
		return vm.startHostNetworkStack(ctx, signalNetworkReady)
	})
	vm.startRunService(&g, ctx, cancelRun, vm.startMachineManagementAPI)

	vm.startRunService(&g, ctx, cancelRun, func(ctx context.Context) error {
		if err := waitForNetworkReady(ctx, networkReady); err != nil {
			return err
		}
		return vm.startPodmanProxy(ctx)
	})

	go func() {
		if err := waitForNetworkReady(ctx, networkReady); err != nil {
			return
		}
		if err := vm.reportPodmanReady(ctx); err != nil {
			logrus.Warnf("podman API readiness check failed: %v", err)
		}
	}()

	go func() {
		vm.WaitAndShutdownMachine(ctx, func() {
			cancelRun(context.Canceled)
		})
	}()

	g.Go(func() error {
		return vm.runVirtualMachine(ctx, cancelRun, networkReady)
	})

	return runError(ctx, g.Wait())
}

func (vm *VM) newNetworkReadySignal() (<-chan struct{}, func()) {
	networkReady := make(chan struct{})
	var once sync.Once

	return networkReady, func() {
		once.Do(func() {
			close(networkReady)
			vm.emit(EventNetworkReady, "host network ready")
		})
	}
}

// waitForNetworkReady blocks until gvproxy reports the host network is usable,
// or returns early when the current VM run is cancelled.
func waitForNetworkReady(ctx context.Context, networkReady <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-networkReady:
		return nil
	}
}

func (vm *VM) startRunService(g *errgroup.Group, ctx context.Context, cancelRun context.CancelCauseFunc, start func(context.Context) error) {
	g.Go(func() error {
		err := start(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}

		cancelRun(err)
		_ = vm.stopVirtualMachine()
		return err
	})
}

// runVirtualMachine waits for the host network to be ready, boots the VM, and
// propagates the VM exit result into the shared run cancellation path.
func (vm *VM) runVirtualMachine(ctx context.Context, cancelRun context.CancelCauseFunc, networkReady <-chan struct{}) error {
	if err := waitForNetworkReady(ctx, networkReady); err != nil {
		return err
	}

	reason := fmt.Errorf("boot virtual machine")
	logrus.Info(reason.Error())
	vm.emit(EventVirtualMachineBooting, reason.Error())

	err := vm.startVirtualMachine(ctx)
	if err != nil {
		cancelRun(err)
	} else {
		cancelRun(context.Canceled)
	}
	return err
}

func runError(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (vm *VM) startPodmanProxy(ctx context.Context) error {
	if vm.machine.RunMode != define.ContainerMode.String() {
		return nil
	}

	switch vm.machine.VirtualNetworkMode {
	case define.GVISOR:
		_, portStr, err := net.SplitHostPort(vm.machine.PodmanInfo.GuestPodmanAPIListenAddr)
		if err != nil {
			return fmt.Errorf("invalid guest podman address %q: %w", vm.machine.PodmanInfo.GuestPodmanAPIListenAddr, err)
		}

		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port in guest podman address %q: %w", portStr, err)
		}

		logrus.Infof("podman proxy listening in %s, forward to %s", vm.machine.PodmanInfo.HostPodmanProxyAddr, vm.machine.PodmanInfo.GuestPodmanAPIListenAddr)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			vm.machine.GVPCtlAddr,
			vm.machine.PodmanInfo.HostPodmanProxyAddr,
			define.GuestIP,
			uint16(port))
	case define.TSI:
		forwarder := &network.LocalForwarder{
			UnixSockAddr: vm.machine.PodmanInfo.HostPodmanProxyAddr,
			Target:       vm.machine.PodmanInfo.GuestPodmanAPIListenAddr,
			Timeout:      time.Second,
		}
		return forwarder.Run(ctx)
	default:
		return fmt.Errorf("unsupported virtual network mode: %s", vm.machine.VirtualNetworkMode)
	}
}

func (vm *VM) startHostNetworkStack(ctx context.Context, onReady func()) error {
	if vm.machine.VirtualNetworkMode == define.TSI {
		onReady()
		return nil
	}

	logrus.Info("starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vm.machine, onReady)
}

func (vm *VM) startIgnitionService(ctx context.Context) error {
	return ignition.NewServer(vm.machine).Start(ctx)
}

func (vm *VM) startMachineManagementAPI(ctx context.Context) error {
	server, err := management.NewServer(vm.provider)
	if err != nil {
		return fmt.Errorf("create management server: %w", err)
	}
	return server.Start(ctx)
}

func (vm *VM) startVirtualMachine(ctx context.Context) error {
	return vm.provider.Start(ctx)
}

func (vm *VM) stopVirtualMachine() error {
	return vm.provider.Stop()
}

func (vm *VM) reportPodmanReady(ctx context.Context) error {
	if err := vm.waitPodmanReady(ctx); err != nil {
		return err
	}

	msg := fmt.Sprintf("podman API proxy ready on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
	logrus.Info(msg)
	vm.emit(EventPodmanReady, msg)
	return nil
}

func (vm *VM) waitPodmanReady(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(vm.machine.PodmanInfo.HostPodmanProxyAddr)
	if err != nil {
		return fmt.Errorf("parse podman proxy address: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, define.DefaultProbeTimeout)
	defer cancel()

	client := network.NewUnixClient(addr.Path, network.WithTimeout(define.DefaultTimeTicker))
	defer client.Close()

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		if pingPodman(ctx, client) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("podman API not ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func pingPodman(ctx context.Context, client *network.Client) bool {
	_, status, err := client.Get("/_ping").DoAndRead(ctx)
	if err != nil {
		return false
	}

	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func (vm *VM) WaitAndShutdownMachine(ctx context.Context, onShutdown func()) {
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
					_ = vm.stopVirtualMachine()
					onShutdown()
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
			_ = vm.stopVirtualMachine()
			onShutdown()
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
