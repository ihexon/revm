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
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// VM represents a virtual machine session.
// Close must always be called to release resources.
type VM struct {
	cfg *Config

	machine          *runtimemachine.Machine
	sessionDir       string
	releaseResources func()
	eventDispatcher  eventDispatcher
	logFile          *os.File

	seq atomic.Uint64
}

// newProvider creates a libkrun Provider for the current platform.
func newProvider(mc *define.MachineSpec) (backend.Backend, error) {
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
	if vm.logFile != nil {
		_ = vm.logFile.Close()
		vm.logFile = nil
	}
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
		cfg:        &normalizedCfg,
		sessionDir: getSessionDir(normalizedCfg.SessionID),
		logFile:    logFile,
	}

	if reporter := newEventReporter(normalizedCfg.ReportURL); reporter != nil {
		vm.eventDispatcher.addReporter(reporter)
	}

	if err := vm.build(ctx); err != nil {
		_ = vm.Close()
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

	machine, err := runtimemachine.New(mc, vmp)
	if err != nil {
		releaseResources()
		return fmt.Errorf("create runtime machine: %w", err)
	}

	vm.machine = machine
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

// Run starts the prepared VM and blocks until it exits.
func (vm *VM) Run(ctx context.Context) error {
	ctx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(context.Canceled)

	networkReady, signalNetworkReady := vm.newNetworkReadySignal()

	var g errgroup.Group

	vm.startRunService(&g, ctx, cancelRun, vm.startIgnitionService)
	vm.startRunService(&g, ctx, cancelRun, func(ctx context.Context) error {
		return vm.startHostNetworkStack(ctx, signalNetworkReady)
	})
	vm.startRunService(&g, ctx, cancelRun, vm.startMachineManagementAPI)

	if err := vm.startModeServices(&g, ctx, cancelRun, networkReady); err != nil {
		return err
	}

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

func (vm *VM) startModeServices(g *errgroup.Group, ctx context.Context, cancelRun context.CancelCauseFunc, networkReady <-chan struct{}) error {
	switch vm.machine.RunMode() {
	case define.RootFsMode.String():
		return nil
	case define.ContainerMode.String():
		vm.startRunService(g, ctx, cancelRun, func(ctx context.Context) error {
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
		return fmt.Errorf("unsupported run mode: %s", vm.machine.RunMode())
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
	if vm.machine.RunMode() != define.ContainerMode.String() {
		return nil
	}

	switch vm.machine.VirtualNetworkMode() {
	case define.GVISOR:
		guestAPIAddr := vm.machine.PodmanGuestAPIListenAddr()
		_, portStr, err := net.SplitHostPort(guestAPIAddr)
		if err != nil {
			return fmt.Errorf("invalid guest podman address %q: %w", guestAPIAddr, err)
		}

		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port in guest podman address %q: %w", portStr, err)
		}

		logrus.Infof("podman proxy listening in %s, forward to %s", vm.machine.PodmanHostProxyAddr(), guestAPIAddr)
		return gvproxy.TunnelHostUnixToGuest(ctx,
			vm.machine.GVPCtlAddr(),
			vm.machine.PodmanHostProxyAddr(),
			define.GuestIP,
			uint16(port))
	default:
		return fmt.Errorf("podman proxy requires %s network, got %s", define.GVISOR, vm.machine.VirtualNetworkMode())
	}
}

func (vm *VM) startHostNetworkStack(ctx context.Context, onReady func()) error {
	if vm.machine.VirtualNetworkMode() == define.TSI {
		onReady()
		return nil
	}

	logrus.Info("starting gvisor-tap-vsock network stack")
	return gvproxy.Run(ctx, vm.machine.GVProxySpec(), onReady)
}

func (vm *VM) startIgnitionService(ctx context.Context) error {
	server, err := ignition.NewServer(vm.machine)
	if err != nil {
		return fmt.Errorf("create ignition server: %w", err)
	}
	return server.Start(ctx)
}

func (vm *VM) startMachineManagementAPI(ctx context.Context) error {
	server, err := management.NewServer(vm.machine)
	if err != nil {
		return fmt.Errorf("create management server: %w", err)
	}
	return server.Start(ctx)
}

func (vm *VM) startVirtualMachine(ctx context.Context) error {
	return vm.machine.Start(ctx)
}

func (vm *VM) stopVirtualMachine() error {
	return vm.machine.Stop()
}

func (vm *VM) reportPodmanReady(ctx context.Context) error {
	if err := vm.waitPodmanReady(ctx); err != nil {
		return err
	}

	msg := fmt.Sprintf("podman API proxy ready on %s", vm.machine.PodmanHostProxyAddr())
	logrus.Info(msg)
	vm.emit(EventPodmanReady, msg)
	return nil
}

func (vm *VM) waitPodmanReady(ctx context.Context) error {
	addr, err := network.ParseUnixAddr(vm.machine.PodmanHostProxyAddr())
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
