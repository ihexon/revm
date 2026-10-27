//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
)

func (vm *VM) GVProxySpec() gvproxy.Spec {
	return gvproxy.Spec{
		ControlAddr:         vm.spec.GVPCtlAddr,
		NetAddr:             vm.spec.GVPVNetAddr,
		NotifyAddr:          vm.spec.GVPNotifyAddr,
		GuestIP:             define.GuestIP,
		HostLoopbackAddress: define.LocalHost,
		PortForwards:        append([]define.PortForward(nil), vm.spec.PortForwards...),
	}
}
