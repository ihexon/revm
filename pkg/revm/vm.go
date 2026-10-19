//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"errors"
	"fmt"
	runtimemachine "linuxvm/internal/machine"
	"linuxvm/pkg/backend"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/network"
	"linuxvm/pkg/podmanproxy"
	"linuxvm/pkg/service/ignition"
	"linuxvm/pkg/service/management"
	"net/http"
	"os"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const defaultForceStopTimeout = 3 * time.Second

// VM owns one prepared virtual machine session.
//
// It deliberately separates three concerns:
//   - runtime: immutable machine view plus backend lifecycle operations.
//   - workspace: host-side session files and their cleanup.
//   - observability: logs and event reporting.
//
// runtimemachine.Machine is only a view derived from the resolved spec. It does
// not start, stop, or mutate the VM; backend.Backend owns those operations.
// Release must always be called to release resources.
type VM struct {
	cfg            *Config
	lease          *sessionRun
	commandContext context.Context

	runtime       vmRuntime
	workspace     vmWorkspace
	observability vmObservability

	seq      atomic.Uint64
	resultMu sync.Mutex
	result   *execution.Result
}

type vmRuntime struct {
	view    *runtimemachine.Machine
	backend backend.Backend
	done    chan struct{}
}

type vmWorkspace struct {
	dir     string
	release func()
}

type vmObservability struct {
	events eventDispatcher
	runLog *os.File
}

// newProvider creates a libkrun Provider for the current platform.
func newProvider(ctx context.Context, mc *define.MachineSpec, locks ...*os.File) (backend.Backend, error) {
	switch {
	case goruntime.GOOS == "darwin" && goruntime.GOARCH == "arm64":
	case goruntime.GOOS == "linux" && (goruntime.GOARCH == "arm64" || goruntime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", goruntime.GOOS, goruntime.GOARCH)
	}
	if err := libkrun.CheckHostSupport(); err != nil {
		return nil, err
	}
	p, err := libkrun.NewProcessProvider(ctx, mc, locks...)
	if err != nil {
		return nil, fmt.Errorf("create libkrun libkrun: %w", err)
	}
	return p, nil
}

// Release frees host-side resources such as the backend, file locks, logs, and event reporters.
// It must always be called, even if Run has not been called. Release is idempotent.
func (vm *VM) Release() error {
	var retErr error
	if vm.runtime.backend != nil {
		if err := vm.runtime.backend.Close(); err != nil {
			// Keep ownership when the VMM is still running. Dropping the backend
			// here would make a later safe retry impossible and could release the
			// workspace while libkrun still references it.
			return err
		}
		vm.runtime.backend = nil
	}
	vm.runtime.view = nil
	if vm.cfg != nil {
		p := newMachinePathManager(vm.workspace.dir)
		for link, target := range map[string]string{vm.cfg.ManageAPIFile: p.GetVMCtlSocketFile(), vm.cfg.PodmanProxyAPIFile: p.GetPodmanSocketFile(), vm.cfg.SSHKeyFileSymbolPath: p.GetSSHKeyFilePath()} {
			if link == "" {
				continue
			}
			if actual, err := os.Readlink(link); err == nil && actual == target {
				retErr = errors.Join(retErr, os.Remove(link))
			}
			if link == vm.cfg.SSHKeyFileSymbolPath {
				if actual, err := os.Readlink(link + ".pub"); err == nil && actual == target+".pub" {
					retErr = errors.Join(retErr, os.Remove(link+".pub"))
				}
			}
		}
	}

	if vm.observability.runLog != nil {
		releaseRunLog(vm.observability.runLog)
		vm.observability.runLog = nil
	}
	if vm.workspace.release != nil {
		vm.workspace.release()
		vm.workspace.release = nil
	}
	vm.observability.events.close()
	if vm.lease != nil {
		retErr = errors.Join(retErr, vm.lease.close())
		vm.lease = nil
	}
	return retErr
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

// Build resolves configuration defaults and acquires the heavyweight resources
// needed to run the VM: workspace lock, SSH keys, rootfs, disks, and provider.
func Build(ctx context.Context, cfg *Config) (retVM *VM, retErr error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}
	if normalizedCfg.RunMode == ModeAttach || normalizedCfg.RunMode == ModeControl {
		return nil, fmt.Errorf("%s mode does not build a VM", normalizedCfg.RunMode)
	}
	lease, err := beginRun(ctx, &normalizedCfg)
	if err != nil {
		return nil, err
	}
	logFile, err := setupRunLogging(normalizedCfg)
	if err != nil {
		lease.finish(nil, err)
		lease.close()
		return nil, fmt.Errorf("setup logging: %w", err)
	}

	logrus.Infof("revm build info: %s", buildTimeInfo())
	logrus.Infof("start virtualMachine, full cmdline: %q", os.Args)

	vm := &VM{
		cfg:   &normalizedCfg,
		lease: lease,
		workspace: vmWorkspace{
			dir: lease.record.RuntimeDir,
		},
		observability: vmObservability{
			runLog: logFile,
		},
	}

	if reporter := newEventReporter(normalizedCfg.ReportURL); reporter != nil {
		vm.observability.events.start(reporter)
	}

	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, lease.finish(nil, retErr), vm.Release())
		}
	}()

	if err := vm.build(ctx); err != nil {
		return nil, err
	}

	return vm, nil
}

// build acquires all heavyweight resources and attaches each one to vm as soon
// as ownership transfers. Build's deferred Release handles failure rollback.
func (vm *VM) build(ctx context.Context) error {
	mc, releaseWorkspace, err := buildMachine(ctx, *vm.cfg, vm.workspace.dir)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}
	vm.workspace.release = releaseWorkspace

	if err := vm.createUserSymlinks(); err != nil {
		return fmt.Errorf("create symlinks: %w", err)
	}

	vmp, err := newProvider(ctx, mc, vm.lease.files...)
	if err != nil {
		return fmt.Errorf("create vm provider: %w", err)
	}
	vm.runtime.backend = vmp

	machine, err := runtimemachine.New(mc)
	if err != nil {
		return fmt.Errorf("create runtime machine: %w", err)
	}
	vm.runtime.view = machine

	return nil
}

// createUserSymlinks links session-internal resources to user-specified paths.
// All actual files remain inside the workspace; the symlinks are just a convenience
// bridge so external tools can find them at well-known locations.
func (vm *VM) createUserSymlinks() error {
	cfg := vm.cfg
	p := newMachinePathManager(vm.workspace.dir)

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

// Run starts the prepared VM, runs all host-side services required by the
// guest, and blocks until the VM exits or the run is aborted.
//
// The run has two cooperating lifetimes:
//   - hostServicesCtx controls services hosted by this process, such as the
//     ignition server, management API, gvproxy stack, and Podman proxy.
//   - the backend owns the blocking VMM call and returns only after the guest
//     exits; host cancellation is handled through ForceStop.
//
// Shutdown is intentionally two-phase. A graceful shutdown request, such as the
// first Ctrl-C or a management API stop request, only asks the guest to exit.
// Host services stay alive and backend.Start keeps waiting because the guest may
// still need management, network, proxy, or SSH-side plumbing while it runs its
// shutdown path. Once the VM exits by itself and backend.Start returns, Run stops
// host services and returns.
//
// A force shutdown request, such as a second Ctrl-C, launcher disappearance, or a
// host service failure, asks the backend to stop the VM, aborts backend.Start's
// wait path, and stops host services so Run can return quickly.
//
// If a host service fails, Run treats it as fatal for the VM session and forces
// the VM run to end. If the VM exits or the run is cancelled normally, Run
// returns nil; otherwise it returns the first meaningful failure cause.
func (vm *VM) Run(ctx context.Context) error {
	vm.commandContext = ctx
	vm.lease.record.Status = "running"
	if err := writeJSONAtomic(vm.lease.resultPath, vm.lease.record); err != nil {
		return err
	}
	vm.runtime.done = make(chan struct{})
	runFinished := make(chan struct{})
	defer close(runFinished)
	hostServicesCtx, stopHostServices := context.WithCancelCause(context.WithoutCancel(ctx))
	defer stopHostServices(context.Canceled)

	finishVMRun := func(cause error) {
		stopHostServices(cause)
	}
	forceVMRun := func(cause error) {
		stopHostServices(cause)
	}
	var forceStopOnce sync.Once
	var forceStopErr error
	forceVMRunWithStop := func(cause error) {
		forceStopOnce.Do(func() {
			forceStopErr = vm.forceVirtualMachine()
		})
		if forceStopErr != nil {
			cause = errors.Join(cause, forceStopErr)
		}
		forceVMRun(cause)
	}

	networkReady, signalNetworkReady := vm.newNetworkReadySignal()

	var g errgroup.Group

	vm.startHostService(&g, hostServicesCtx, forceVMRunWithStop, vm.startIgnitionService)
	vm.startHostService(&g, hostServicesCtx, forceVMRunWithStop, func(ctx context.Context) error {
		return vm.startHostNetworkStack(ctx, signalNetworkReady)
	})
	vm.startHostService(&g, hostServicesCtx, forceVMRunWithStop, vm.startMachineManagementAPI)

	if err := vm.startModeServices(&g, hostServicesCtx, forceVMRunWithStop, networkReady); err != nil {
		return err
	}

	go func() {
		select {
		case <-ctx.Done():
		case <-runFinished:
			return
		}
		if vm.cfg.RunMode == ModeContainer {
			vm.requestGuestShutdown()
		}
		grace := vm.stopBudget(ctx)
		timer := time.NewTimer(grace)
		defer timer.Stop()
		var force <-chan struct{}
		if c := execution.ControllerFrom(ctx); c != nil {
			force = c.Force()
		}
		select {
		case <-vm.runtime.done:
			return
		case <-runFinished:
			return
		case <-timer.C:
		case <-force:
		}
		forceVMRunWithStop(context.Cause(ctx))
	}()

	g.Go(func() error {
		if err := waitForNetworkReady(hostServicesCtx, networkReady); err != nil {
			return err
		}

		reason := fmt.Errorf("boot virtual machine")
		logrus.Info(reason.Error())
		vm.emit(EventVirtualMachineBooting, reason.Error())

		// libkrun_vmm_run is a blocking C call. The backend intentionally owns
		// that lifetime and returns only after the guest exits.
		err := vm.runtime.backend.Start(context.Background())
		close(vm.runtime.done)
		if err != nil {
			finishVMRun(err)
		} else {
			finishVMRun(context.Canceled)
		}
		return err
	})

	err := runError(hostServicesCtx, g.Wait())
	if vm.cfg.RunMode == ModeRootfs {
		vm.resultMu.Lock()
		defer vm.resultMu.Unlock()
		if vm.result == nil {
			return errors.Join(err, fmt.Errorf("VM stopped without a command result; exit status unknown"))
		}
		var exit *execution.ExitError
		if errors.As(err, &exit) {
			return err
		}
		return errors.Join(vm.result.Err(), err)
	}
	return err
}

func (vm *VM) requestGuestShutdown() {
	if vm.runtime.backend == nil {
		return
	}
	if err := vm.runtime.backend.RequestShutdown(context.Background()); err != nil {
		logrus.Warnf("request guest shutdown failed: %v", err)
	}
}

func (vm *VM) forceVirtualMachine() error {
	if vm.runtime.backend == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultForceStopTimeout)
	defer cancel()

	if err := vm.runtime.backend.ForceStop(ctx); err != nil {
		logrus.Warnf("force stop virtual machine failed: %v", err)
		return err
	}
	return nil
}

func (vm *VM) startModeServices(g *errgroup.Group, ctx context.Context, forceVMRun func(error), networkReady <-chan struct{}) error {
	switch vm.runtime.view.RunMode() {
	case define.RootFsMode.String():
		g.Go(func() error {
			result, err := executeGuest(vm.commandContext, vm.runtime.view.GuestControlTarget(), *vm.cfg)
			vm.resultMu.Lock()
			vm.result = &result
			vm.resultMu.Unlock()
			vm.requestGuestShutdown()
			select {
			case <-vm.runtime.done:
			case <-time.After(vm.stopBudget(vm.commandContext)):
				err = errors.Join(err, fmt.Errorf("VM graceful shutdown timed out"), vm.forceVirtualMachine())
			}
			return err
		})
		return nil
	case define.ContainerMode.String():
		vm.startHostService(g, ctx, forceVMRun, func(ctx context.Context) error {
			socket, err := network.ParseUnixAddr(vm.runtime.view.PodmanHostProxyAddr())
			if err != nil {
				return err
			}
			transport, err := network.ParseUnixAddr(vm.runtime.view.PodmanHostTransportAddr())
			if err != nil {
				return err
			}
			return podmanproxy.ServeHost(ctx, socket.Path, transport.Path)
		})
		go func() {
			if err := waitForNetworkReady(ctx, networkReady); err != nil {
				return
			}
			if err := vm.reportPodmanReady(ctx); err != nil {
				logrus.Warnf("podman API readiness check failed: %v", err)
			}
		}()

		return nil
	default:
		return fmt.Errorf("unsupported run mode: %s", vm.runtime.view.RunMode())
	}
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

func (vm *VM) startHostService(g *errgroup.Group, ctx context.Context, forceVMRun func(error), start func(context.Context) error) {
	g.Go(func() error {
		err := start(ctx)
		if err == nil || errors.Is(err, context.Canceled) {
			return err
		}

		forceVMRun(err)
		return err
	})
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

func (vm *VM) startHostNetworkStack(ctx context.Context, onReady func()) error {
	if vm.runtime.view.VirtualNetworkMode() == define.TSI {
		onReady()
		return nil
	}

	logrus.Info("starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vm.runtime.view.GVProxySpec(), onReady)
}

func (vm *VM) startIgnitionService(ctx context.Context) error {
	server, err := ignition.NewServer(vm.runtime.view)
	if err != nil {
		return fmt.Errorf("create ignition server: %w", err)
	}
	return server.Start(ctx)
}

func (vm *VM) startMachineManagementAPI(ctx context.Context) error {
	server, err := management.NewServer(managementMachine{
		Machine: vm.runtime.view,
		backend: vm.runtime.backend,
	})
	if err != nil {
		return fmt.Errorf("create management server: %w", err)
	}
	return server.Start(ctx)
}

type managementMachine struct {
	*runtimemachine.Machine
	backend backend.Backend
}

func (m managementMachine) RequestShutdown(ctx context.Context) error {
	return m.backend.RequestShutdown(ctx)
}

func (vm *VM) reportPodmanReady(ctx context.Context) error {
	if err := vm.waitPodmanReady(ctx); err != nil {
		return err
	}

	msg := fmt.Sprintf("podman API proxy ready on %s", vm.runtime.view.PodmanHostProxyAddr())
	logrus.Info(msg)
	vm.emit(EventPodmanReady, msg)
	return nil
}

func (vm *VM) waitPodmanReady(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(vm.runtime.view.PodmanHostProxyAddr())
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

func (vm *VM) stopBudget(ctx context.Context) time.Duration {
	if c := execution.ControllerFrom(ctx); c != nil {
		if cause := c.Cause(); cause != nil {
			return max(0, time.Until(cause.Deadline))
		}
	}
	return vm.cfg.StopTimeout
}

func (vm *VM) emitStopping(reason string) {
	logrus.Info(reason)
	vm.emit(EventStopping, reason)
}

// emit sending events with option msg
func (vm *VM) emit(kind EventKind, msg string) {
	if vm == nil || vm.cfg == nil {
		return
	}
	vm.observability.events.emit(vm.cfg.SessionID, vm.cfg.RunMode, kind, msg, vm.seq.Add(1))
}
