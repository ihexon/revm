//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"errors"
	"fmt"
	"io"
	runtimemachine "linuxvm/internal/machine"
	"linuxvm/pkg/backend"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/ignition"
	"linuxvm/pkg/service/management"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
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
	cfg *Config

	runtime       vmRuntime
	workspace     vmWorkspace
	observability vmObservability

	seq atomic.Uint64
}

type vmRuntime struct {
	view    *runtimemachine.Machine
	backend backend.Backend
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
func newProvider(mc *define.MachineSpec) (backend.Backend, error) {
	switch {
	case goruntime.GOOS == "darwin" && goruntime.GOARCH == "arm64":
	case goruntime.GOOS == "linux" && (goruntime.GOARCH == "arm64" || goruntime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", goruntime.GOOS, goruntime.GOARCH)
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

// Release frees host-side resources such as file locks, logs, and event reporters.
// It must always be called, even if Run has not been called. Release is idempotent.
func (vm *VM) Release() error {
	if vm.observability.runLog != nil {
		logrus.SetOutput(os.Stderr)
		_ = vm.observability.runLog.Close()
		vm.observability.runLog = nil
	}
	if vm.workspace.release != nil {
		vm.workspace.release()
		vm.workspace.release = nil
	}
	vm.observability.events.close()
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

// Build resolves configuration defaults and acquires the heavyweight resources
// needed to run the VM: workspace lock, SSH keys, rootfs, disks, and provider.
func Build(ctx context.Context, cfg *Config) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}
	if normalizedCfg.RunMode == ModeAttach {
		return nil, fmt.Errorf("attach mode does not build a VM; use Attach")
	}

	setupLogrus(normalizedCfg.LogLevel)

	logFile, err := setupLogFile(normalizedCfg)
	if err != nil {
		return nil, fmt.Errorf("setup logging: %w", err)
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, logFile))
	logrus.Infof("revm build info: %s", buildTimeInfo())
	logrus.Infof("start virtualMachine, full cmdline: %q", os.Args)

	vm := &VM{
		cfg: &normalizedCfg,
		workspace: vmWorkspace{
			dir: getSessionDir(normalizedCfg.SessionID),
		},
		observability: vmObservability{
			runLog: logFile,
		},
	}

	if reporter := newEventReporter(normalizedCfg.ReportURL); reporter != nil {
		vm.observability.events.addReporter(reporter)
	}

	if err := vm.build(ctx); err != nil {
		_ = vm.Release()
		return nil, err
	}

	return vm, nil
}

func setupLogrus(level string) {
	if level == "" {
		level = logrus.InfoLevel.String()
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		l = logrus.InfoLevel
		logrus.Warnf("failed to parse log level: %v, using default log level %s", err, l.String())
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})
	logrus.SetOutput(os.Stderr)
}

func setupLogFile(cfg Config) (*os.File, error) {
	logFilePath := cfg.LogTo
	if logFilePath == "" {
		logFilePath = filepath.Join(getSessionDir(cfg.SessionID), "logs", "revm.log")
	}

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogFileSize {
		if err := os.Truncate(logFilePath, 0); err != nil {
			return nil, fmt.Errorf("truncate log file: %w", err)
		}
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}

// build acquires all heavyweight resources. On failure it cleans up after itself.
func (vm *VM) build(ctx context.Context) error {
	mc, releaseWorkspace, err := buildMachine(ctx, *vm.cfg, vm.workspace.dir)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}

	if err := vm.createUserSymlinks(); err != nil {
		releaseWorkspace()
		return fmt.Errorf("create symlinks: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		releaseWorkspace()
		return fmt.Errorf("create vm provider: %w", err)
	}

	machine, err := runtimemachine.New(mc)
	if err != nil {
		releaseWorkspace()
		return fmt.Errorf("create runtime machine: %w", err)
	}

	vm.runtime = vmRuntime{
		view:    machine,
		backend: vmp,
	}
	vm.workspace.release = releaseWorkspace
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
//   - vmWaitAbortCtx is passed to backend.Start and only controls whether the
//     host keeps waiting for the VM exit path to return.
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
	hostServicesCtx, stopHostServices := context.WithCancelCause(ctx)
	defer stopHostServices(context.Canceled)

	vmWaitAbortCtx, abortVMWait := context.WithCancelCause(ctx)
	defer abortVMWait(context.Canceled)

	finishVMRun := func(cause error) {
		stopHostServices(cause)
	}
	forceVMRun := func(cause error) {
		abortVMWait(cause)
		stopHostServices(cause)
	}

	networkReady, signalNetworkReady := vm.newNetworkReadySignal()

	var g errgroup.Group

	vm.startHostService(&g, hostServicesCtx, forceVMRun, vm.startIgnitionService)
	vm.startHostService(&g, hostServicesCtx, forceVMRun, func(ctx context.Context) error {
		return vm.startHostNetworkStack(ctx, signalNetworkReady)
	})
	vm.startHostService(&g, hostServicesCtx, forceVMRun, vm.startMachineManagementAPI)

	if err := vm.startModeServices(&g, hostServicesCtx, forceVMRun, networkReady); err != nil {
		return err
	}

	forceHostShutdown := func() {
		vm.forceVirtualMachine()
		forceVMRun(context.Canceled)
	}

	vm.startShutdownMonitors(hostServicesCtx, vm.requestGuestShutdown, forceHostShutdown)

	g.Go(func() error {
		if err := waitForNetworkReady(hostServicesCtx, networkReady); err != nil {
			return err
		}

		reason := fmt.Errorf("boot virtual machine")
		logrus.Info(reason.Error())
		vm.emit(EventVirtualMachineBooting, reason.Error())

		err := vm.runtime.backend.Start(vmWaitAbortCtx)
		if err != nil {
			finishVMRun(err)
		} else {
			finishVMRun(context.Canceled)
		}
		return err
	})

	return runError(hostServicesCtx, g.Wait())
}

func (vm *VM) requestGuestShutdown() {
	if err := vm.runtime.backend.RequestShutdown(context.Background()); err != nil {
		logrus.Warnf("request guest shutdown failed: %v", err)
	}
}

func (vm *VM) forceVirtualMachine() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultForceStopTimeout)
	defer cancel()

	if err := vm.runtime.backend.ForceStop(ctx); err != nil {
		logrus.Warnf("force stop virtual machine failed: %v", err)
	}
}

func (vm *VM) startModeServices(g *errgroup.Group, ctx context.Context, forceVMRun func(error), networkReady <-chan struct{}) error {
	switch vm.runtime.view.RunMode() {
	case define.RootFsMode.String():
		return nil
	case define.ContainerMode.String():
		vm.startHostService(g, ctx, forceVMRun, func(ctx context.Context) error {
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

		vm.forceVirtualMachine()
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

func (vm *VM) startPodmanProxy(ctx context.Context) error {
	if vm.runtime.view.RunMode() != define.ContainerMode.String() {
		return nil
	}

	switch vm.runtime.view.VirtualNetworkMode() {
	case define.GVISOR:
		guestAPIAddr := vm.runtime.view.PodmanGuestAPIListenAddr()
		_, portStr, err := net.SplitHostPort(guestAPIAddr)
		if err != nil {
			return fmt.Errorf("invalid guest podman address %q: %w", guestAPIAddr, err)
		}

		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port in guest podman address %q: %w", portStr, err)
		}

		logrus.Infof("podman proxy listening in %s, forward to %s", vm.runtime.view.PodmanHostProxyAddr(), guestAPIAddr)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			vm.runtime.view.GVPCtlAddr(),
			vm.runtime.view.PodmanHostProxyAddr(),
			define.GuestIP,
			uint16(port))
	default:
		return fmt.Errorf("podman proxy requires %s network, got %s", define.GVISOR, vm.runtime.view.VirtualNetworkMode())
	}
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

func (vm *VM) startShutdownMonitors(ctx context.Context, requestGuestShutdown, forceHostShutdown func()) {
	// Force shutdown if the launcher disappears and can no longer own cleanup.
	go vm.monitorLauncherExit(ctx, forceHostShutdown)

	go vm.monitorInterrupts(ctx, requestGuestShutdown, forceHostShutdown)
}

func (vm *VM) monitorLauncherExit(ctx context.Context, forceHostShutdown func()) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if os.Getppid() == 1 {
				vm.emitStopping("parent process exited, shutting down machine")
				forceHostShutdown()
				return
			}
		}
	}
}

func (vm *VM) monitorInterrupts(ctx context.Context, requestGuestShutdown, forceHostShutdown func()) {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	firstSignal, ok := waitForShutdownSignal(ctx, sigCh)
	if !ok {
		return
	}
	vm.emitStopping("received signal, shutting down")
	requestGuestShutdown()
	logrus.Warnf("waiting for guest shutdown after %s; press Ctrl-C again to force shutdown", firstSignal)

	secondSignal, ok := waitForShutdownSignal(ctx, sigCh)
	if !ok {
		return
	}
	logrus.Warnf("received %s again, forcing shutdown", secondSignal)
	forceHostShutdown()
}

func waitForShutdownSignal(ctx context.Context, sigCh <-chan os.Signal) (os.Signal, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case sig := <-sigCh:
		return sig, true
	}
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
