//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <stdlib.h>
*/
import "C"

import (
	"linuxvm/pkg/define"
	"runtime"
)

// ExitProcess is called only by main, after it records the command result and
// restores the terminal. It releases native threads without running C teardown.
func ExitProcess(code int) { C._Exit(C.int(code)) }

// Run consumes this process. The CLI exits after the workload completes or is
// canceled; it never destroys a native VMM concurrently with krun_vmm_run.
func Run(spec *define.MachineSpec) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	vm := &machine{cfg: spec}
	if err := vm.create(); err != nil {
		return err
	}
	return vm.start()
}
