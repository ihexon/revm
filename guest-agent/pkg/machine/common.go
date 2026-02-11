package machine

import "linuxvm/pkg/define"

type Machine define.VMConfig

func (m *Machine) GetVirtualNetworkType() define.VNetMode {
	if m.VirtualNetworkMode == define.TSI.String() {
		return define.TSI
	}

	return define.GVISOR
}


