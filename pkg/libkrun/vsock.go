//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"

	"github.com/sirupsen/logrus"
)

func (v *Libkrun) setupVSock() error {
	var features C.uint32_t
	if v.cfg.VirtualNetworkMode == define.TSI {
		features = C.KRUN_TSI_FLAGS_HIJACK_INET
	}
	var errOut C.KrunError
	device := C.krun_vsock_device_new(C.uint64_t(vsockCID), features, &errOut)
	if err := checkKrunHandle("create vsock device", device, errOut); err != nil {
		return err
	}
	addr, err := network.ParseUnixAddr(v.cfg.IgnitionServerCfg.ListenSockAddr)
	if err != nil {
		C.krun_vsock_device_destroy(device)
		return err
	}
	path := newKrunStr(addr.Path)
	defer path.free()
	C.krun_vsock_device_add_unix_port(device, C.uint32_t(define.DefaultVSockPort), path.value, C.bool(false))
	C.krun_mmio_device_manager_add(v.manager, device)
	logrus.Infof("vsock port %d → %s", define.DefaultVSockPort, addr.Path)
	return nil
}
