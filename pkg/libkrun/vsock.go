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

func (v *machine) setupVSock() error {
	var features C.uint32_t
	if v.cfg.Network == define.NetworkTSI {
		// Intercept IPv4/IPv6 sockets. Unix sockets remain inside the guest.
		features = C.KRUN_TSI_FLAGS_HIJACK_INET
	}
	var errOut C.KrunError
	device := C.krun_vsock_device_new(C.uint64_t(define.GuestVSockCID), features, &errOut)
	if err := checkKrunHandle("create vsock device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	controlAddr, err := network.ParseUnixAddr(v.cfg.GuestControlAddr)
	if err != nil {
		return err
	}
	controlPath := newKrunStr(controlAddr.Path)
	defer controlPath.free()
	// The control endpoint is host-initiated, so libkrun owns the host-side
	// Unix listener and forwards accepted connections into the guest port.
	C.krun_vsock_device_add_unix_port(device, C.uint32_t(define.GuestControlPort), controlPath.value, C.bool(true))
	if v.cfg.RunMode == define.ContainerMode {
		podmanAddr, err := network.ParseUnixAddr(v.cfg.PodmanInfo.HostPodmanTransportAddr)
		if err != nil {
			return err
		}
		podmanPath := newKrunStr(podmanAddr.Path)
		defer podmanPath.free()
		C.krun_vsock_device_add_unix_port(device, C.uint32_t(define.GuestPodmanPort), podmanPath.value, C.bool(true))
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	logrus.Infof("vsock port %d → %s", define.GuestControlPort, controlAddr.Path)
	return nil
}
