//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/krunrunner"
	"linuxvm/pkg/service/lifecycle"
	sshsvc "linuxvm/pkg/service/ssh"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// VM represents a running (or ready-to-run) virtual machine.
// Close must always be called to release resources.
type VM struct {
	cfg *Config

	machine         *define.Machine
	provider        interfaces.VMMProvider
	svc             hostServices
	sessionDir      string
	cleanup         func()
	eventDispatcher eventDispatcher
	Cancel          context.CancelFunc

	mu    sync.Mutex
	state vmState

	seq atomic.Uint64
}

// newProvider creates a RunnerProvider for the current platform, delegating
// all libkrun CGo calls to a child process to prevent libkrun's exit() from
// terminating the main process before cleanup can run.
func newProvider(mc *define.Machine) (*krunrunner.RunnerProvider, error) {
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
	case runtime.GOOS == "linux" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return krunrunner.NewRunnerProvider(mc), nil
}

// vmState 表示 VM 的生命周期状态（单调递增）。
type vmState uint8

const (
	vmStateNew      vmState = iota // New() 成功后的初始状态
	vmStateRunning                 // Run() 正在执行中
	vmStateStopping                // Stop() 已被调用
	vmStateStopped                 // Run() 已返回
	vmStateClosed                  // Close() 已被调用
)

// Close 释放所有资源（文件锁、workspace 目录、event eventDispatcher）。
// 必须始终调用，即使 Run() 从未被调用。幂等。
func (vm *VM) Close() error {
	vm.mu.Lock()
	if vm.state == vmStateClosed {
		vm.mu.Unlock()
		return nil
	}
	vm.state = vmStateClosed
	vm.mu.Unlock()

	if vm.cleanup != nil {
		vm.cleanup()
	}
	vm.eventDispatcher.close()
	return nil
}

type hostServices interface {
	StartPodmanProxy(ctx context.Context) error
	StartNetworkStack(ctx context.Context) error
	StartIgnitionService(ctx context.Context) error
	StartMachineManagementAPI(ctx context.Context, stopFn func()) error
	StartVirtualMachine(ctx context.Context) error
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
		state:      vmStateNew,
		Cancel:     nil,
	}

	for _, r := range cfg.Reporters {
		vm.eventDispatcher.addReporter(r)
	}

	return vm, nil
}

// init acquires all heavyweight resources: workspace dirs, flock, SSH keys,
// disk images, krun-runner provider, and host services. Called once at the
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
	vm.svc = lifecycle.NewHostServices(mc, vmp)
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
	if cfg.SSHKeyDir != "" {
		if err := createSymlink(filepath.Dir(p.GetSSHPrivateKeyFile()), cfg.SSHKeyDir); err != nil {
			return fmt.Errorf("ssh key dir: %w", err)
		}
	}
	if cfg.ExportSSHKeyPrivateFile != "" {
		if err := createSymlink(p.GetSSHPrivateKeyFile(), cfg.ExportSSHKeyPrivateFile); err != nil {
			return fmt.Errorf("ssh private key: %w", err)
		}
	}
	if cfg.ExportSSHKeyPublicFile != "" {
		if err := createSymlink(p.GetSSHPrivateKeyFile()+".pub", cfg.ExportSSHKeyPublicFile); err != nil {
			return fmt.Errorf("ssh public key: %w", err)
		}
	}
	return nil
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

	if err := vm.init(ctx); err != nil {
		return err
	}

	switch vm.cfg.RunMode {
	case ModeRootfs, ModeContainer:
	default:
		return fmt.Errorf("unsupported run mode %q", vm.cfg.RunMode)
	}

	vm.emit(EventVMStarting, "starting vm")

	// go vm.orphanMonitor(ctx)

	// --- host services errgroup ---
	g, ctx := errgroup.WithContext(ctx)

	// start ignition server
	g.Go(func() error {
		return vm.svc.StartIgnitionService(ctx)
	})

	// start vnet
	g.Go(func() error {
		return vm.svc.StartNetworkStack(ctx)
	})

	// start virtual machine management API
	g.Go(func() error {
		return vm.svc.StartMachineManagementAPI(ctx, vm.Cancel)
	})

	// start podman api proxy
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vm.machine.Readiness.VNetHostReady:
			return vm.svc.StartPodmanProxy(ctx)
		}
	})

	// send podman ready
	go func() {
		if vm.cfg.RunMode == ModeContainer {
			select {
			case <-ctx.Done():
				return
			case <-vm.machine.Readiness.PodmanReady:
				vm.emit(EventPodmanReady, fmt.Sprintf("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr))
				logrus.Infof("podman API proxy ready on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
				return
			}
		}
	}()

	// send ssh ready
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-vm.machine.Readiness.SSHReady:
			vm.emit(EventSSHReady, "ssh ready")
			return
		}
	}()

	// send host vnet ready
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-vm.machine.Readiness.VNetHostReady:
			vm.emit(EventNetworkReady, "host network ready")
			return
		}
	}()

	go func() {
		vm.WaitAndShutdownMachine(vm.Cancel)
	}()

	svcErrCh := make(chan error, 1)
	go func() {
		svcErrCh <- g.Wait()
		close(svcErrCh)
	}()

	select {
	case <-ctx.Done():
		return <-svcErrCh
	case <-vm.machine.Readiness.VNetHostReady:
		err := vm.svc.StartVirtualMachine(context.Background()) // block
		vm.Cancel()
		<-svcErrCh // wait for svc group to finish
		return err
	}
}

func (vm *VM) WaitAndShutdownMachine(cancel context.CancelFunc) {
	go func() {
		for {
			if os.Getppid() == 1 {
				logrus.Info("parent process exited, shutting down machine")
				_ = vm.provider.Stop()
				cancel()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// shutdown when signal
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logrus.Info("received signal, shutting down")
		_ = vm.provider.Stop()
		cancel()
	}()
}

// execIgnoreErr runs a command in the VM via SSH, logging but not returning errors.
func (vm *VM) execIgnoreErr(ctx context.Context, cmdline ...string) {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine)
	if err != nil {
		logrus.Warnf("ssh connect for %v: %v", cmdline, err)
		return
	}
	defer client.Close()

	if err := client.Run(ctx, shellescape.QuoteCommand(cmdline)); err != nil {
		logrus.Warnf("exec %v: %v", cmdline, err)
	}
}

func GenerateVMConfig(ctx context.Context, cfg *Config, path string) error {
	vm := &VM{
		cfg: cfg,
	}

	for _, r := range cfg.Reporters {
		vm.eventDispatcher.addReporter(r)
	}
	defer vm.emit(EventExit, "")

	if err := cfg.WriteCfg(path); err != nil {
		vm.emit(EventError, err.Error())
		return err
	}

	vm.emit(EventSuccess, "")

	return nil
}
