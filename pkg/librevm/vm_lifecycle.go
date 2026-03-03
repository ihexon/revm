//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
)

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
