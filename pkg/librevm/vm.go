//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"sync"
	"sync/atomic"
)

// VM represents a running (or ready-to-run) virtual machine.
// Close must always be called to release resources.
type VM struct {
	cfg *Config

	machine       *define.Machine
	provider      interfaces.VMMProvider
	svc           hostServices
	workspacePath string
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
