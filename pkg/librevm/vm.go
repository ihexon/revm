//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/krunrunner"
	"linuxvm/pkg/service/lifecycle"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

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

	mu    sync.Mutex
	state vmState

	seq     atomic.Uint64
	stopper *stopController
}

type stopController struct {
	machine *define.Machine
}

func newStopController(machine *define.Machine) *stopController {
	return &stopController{machine: machine}
}

func (s *stopController) Request() {
	if s == nil || s.machine == nil {
		return
	}
	s.machine.StopOnce.Do(func() { close(s.machine.StopCh) })
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

// Stop 发出停止信号，触发 VM 优雅关机。幂等，多次调用安全。
func (vm *VM) Stop(ctx context.Context) error {
	vm.mu.Lock()
	if vm.state >= vmStateStopping {
		vm.mu.Unlock()
		return nil
	}
	vm.state = vmStateStopping
	vm.mu.Unlock()

	vm.emit(EventStopping, "stopping vm")
	if vm.provider != nil {
		_ = vm.provider.Stop(ctx) // 先杀 krun-runner
	}
	vm.requestStopOtherServices() // 再通知其他服务关闭
	return nil
}

// requestStopOtherServices 委托 stopController 关闭 StopCh 通道（once-safe）。
func (vm *VM) requestStopOtherServices() {
	vm.stopper.Request()
}

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
	ExitVirtualMachineWhenSomethingHappened(ctx context.Context) error
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
	vm.stopper = newStopController(mc)
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
		return vm.run(ctx)
	default:
		return fmt.Errorf("unsupported run mode %q", vm.cfg.RunMode)
	}
}

func (vm *VM) run(ctx context.Context) error {
	vm.emit(EventVMStarting, "starting vm")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return vm.svc.ExitVirtualMachineWhenSomethingHappened(ctx) })
	g.Go(func() error {
		vm.emit(EventIgnitionStarting, "starting ignition service")
		return vm.svc.StartIgnitionService(ctx)
	})
	g.Go(func() error {
		vm.emit(EventNetworkStarting, "starting network stack")
		return vm.svc.StartNetworkStack(ctx)
	})
	g.Go(func() error {
		vm.emit(EventManagementStarting, "starting management API")
		return vm.svc.StartMachineManagementAPI(ctx, func() { _ = vm.Stop(context.Background()) })
	})

	switch vm.cfg.RunMode {
	case ModeContainer:
		g.Go(func() error {
			go func() {
				select {
				case <-ctx.Done():
					return
				case <-vm.machine.Readiness.PodmanReady:
					vm.emit(EventPodmanReady, fmt.Sprintf("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr))
					logrus.Infof("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
				}
			}()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-vm.machine.Readiness.VNetHostReady:
				vm.emit(EventPodmanProxyStarting, "starting podman proxy")
				return vm.svc.StartPodmanProxy(ctx)
			}
		})
	case ModeRootfs:
	}

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vm.machine.Readiness.SSHReady:
			vm.emit(EventSSHReady, "ssh ready")
		}
		return nil
	})

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-vm.machine.Readiness.VNetHostReady:
		vm.emit(EventNetworkReady, "host network ready")
	}

	// When host services request shutdown (e.g. parent process exit), kill the VM process.
	go func() {
		<-ctx.Done()
		_ = vm.Stop(context.Background())
	}()

	// all services ready, now we can start libkrun runner
	vmErr := vm.svc.StartVirtualMachine(ctx)

	go func() {
		if svcErr := g.Wait(); svcErr != nil {
			logrus.Infof("host service error after krun runner exited: %v", svcErr)
		}
	}()

	if vmErr != nil {
		vm.emit(EventError, vmErr.Error())
	}
	_ = vm.Stop(context.Background())
	vm.emit(EventStopped, "vm stopped")
	return vmErr
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
