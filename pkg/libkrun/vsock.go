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
	"unsafe"

	"github.com/sirupsen/logrus"
)

func (v *Libkrun) setupVSock() error {
	var features C.uint32_t
	if v.cfg.VirtualNetworkMode == define.TSI {
		// Upstream's HIJACK_INET flag covers both AF_INET and AF_INET6.
		// AF_UNIX remains intentionally disabled; it is a separate semantic
		// choice and should not change the guest's Unix socket namespace.
		features = C.KRUN_TSI_FLAGS_HIJACK_INET
	}
	var errOut C.KrunError
	device := C.krun_vsock_device_new(C.uint64_t(vsockCID), features, &errOut)
	if err := checkKrunHandle("create vsock device", unsafe.Pointer(device), errOut); err != nil {
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
	controlAddr, err := network.ParseUnixAddr(v.cfg.GuestControlAddr)
	if err != nil {
		C.krun_vsock_device_destroy(device)
		return err
	}
	controlPath := newKrunStr(controlAddr.Path)
	defer controlPath.free()
	// The control endpoint is host-initiated, so libkrun owns the host-side
	// Unix listener and forwards accepted connections into the guest port.
	C.krun_vsock_device_add_unix_port(device, C.uint32_t(define.GuestControlPort), controlPath.value, C.bool(true))
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	logrus.Infof("vsock port %d → %s", define.DefaultVSockPort, addr.Path)
	logrus.Infof("vsock port %d → %s", define.GuestControlPort, controlAddr.Path)
	return nil
}
