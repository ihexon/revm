//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import "linuxvm/pkg/define"

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
