//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/krunrunner"
	"linuxvm/pkg/service/lifecycle"
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
	workspacePath   string
	cleanup         func()
	eventDispatcher eventDispatcher

	mu      sync.Mutex
	state   vmState
	seq     atomic.Uint64
	stopper *stopController
}

// Workspace returns the workspace directory path.
func (vm *VM) Workspace() string {
	return vm.workspacePath
}

// WriteJSONFile marshals the config as JSON and writes it to path.

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

// New creates a VM from the given Config. It resolves defaults, validates the
// configuration, sets up the workspace and file lock, builds the internal
// Machine, and prepares the VM provider.
//
// Close must be called even if Start is never called.
func New(ctx context.Context, cfg *Config) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}
	workspacePath := getWorkspacePath(normalizedCfg.SessionID)

	mc, cleanup, err := buildMachine(ctx, normalizedCfg, workspacePath)
	if err != nil {
		return nil, fmt.Errorf("build machine: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create vm provider: %w", err)
	}

	vm := &VM{
		cfg:           &normalizedCfg,
		machine:       mc,
		provider:      vmp,
		svc:           lifecycle.NewHostServices(mc, vmp),
		workspacePath: workspacePath,
		cleanup:       cleanup,
		state:         vmStateNew,
		stopper:       newStopController(mc),
		eventDispatcher: eventDispatcher{
			proxy: func(sink SinkKind, evt Event) Event {
				if sink != SinkLegacy {
					return evt
				}
				switch evt.Kind {
				case EventStopped:
					evt.Kind = "Exit"
				case EventPodmanReady:
					evt.Kind = "Ready"
				case EventError:
					evt.Kind = "Error"
				case EventSuccess:
					evt.Kind = "Success"
				}
				return evt
			},
		},
	}

	if normalizedCfg.V1EventReportURL != "" {
		vm.registerV1EventSink()
	}
	if normalizedCfg.LegacyEventReportURL != "" {
		vm.registerLegacyEventSink()
	}

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

	switch vm.cfg.RunMode {
	case ModeRootfs, ModeContainer:
		return vm.run(ctx)
	default:
		return fmt.Errorf("unsupported run mode %q", vm.cfg.RunMode)
	}
}

func (vm *VM) run(ctx context.Context) error {
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

	switch vm.cfg.RunMode {
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
		return context.Cause(gctx)
	case <-vm.machine.Readiness.VNetHostReady:
		vm.emit(EventNetworkReady, "host network ready")
	}

	// all thins ready , now we can start libkrun runner
	vmErr := vm.svc.StartVirtualMachine(ctx)
	go func() {
		if vmErr != nil {
			logrus.Infof("host service error after krun runner started: %v", g.Wait())
		}
	}()

	if vmErr != nil {
		vm.emit(EventError, vmErr.Error())
	}
	vm.requestStopOtherServices()
	vm.emit(EventStopped, "vm stopped")
	return vmErr
}
