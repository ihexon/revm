//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"unsafe"

	"github.com/sirupsen/logrus"
)

var guestMAC = [6]byte{0x5a, 0x94, 0xef, 0xe4, 0x0c, 0xee}

// Keep the feature set expected by gvproxy's vfkit transport. libkrun 2.x
// accepts the virtio-net feature bits directly instead of exporting the old
// COMPAT_NET_FEATURES macro.
const compatNetFeatures = (1 << 0) | (1 << 1) | (1 << 7) | (1 << 10) | (1 << 11) | (1 << 14)

func (v *Libkrun) setupNetwork() error {
	switch v.cfg.VirtualNetworkMode {
	case define.GVISOR:
		return v.setupGVisor()
	case define.TSI:
		logrus.Info("configuring TSI network")
		return nil
	default:
		return fmt.Errorf("unknown network mode: %s", v.cfg.VirtualNetworkMode)
	}
}

func (v *Libkrun) setupGVisor() error {
	logrus.Info("configuring gvisor-tap-vsock network")
	addr, err := network.ParseUnixAddr(v.cfg.GVPVNetAddr)
	if err != nil {
		return err
	}
	// libkrun's device registry uses stable netN identifiers. The guest still
	// receives the first virtio-net device as eth0.
	id := newKrunStr("net0")
	path := newKrunStr(addr.Path)
	defer id.free()
	defer path.free()
	var macData [6]C.uint8_t
	for i, b := range guestMAC {
		macData[i] = C.uint8_t(b)
	}
	mac := C.KrunBytes{data: &macData[0], len: C.size_t(len(macData))}
	var errOut C.KrunError
	device := C.krun_net_device_new_unixgram_path(id.value, path.value, mac, C.uint32_t(compatNetFeatures), C.KRUN_NET_FLAGS_VFKIT, &errOut)
	if err := checkKrunHandle("create gvisor network device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}
