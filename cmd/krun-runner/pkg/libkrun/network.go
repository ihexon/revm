//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <libkrun.h>
*/
import "C"

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"runtime"

	"github.com/sirupsen/logrus"
)

var guestMAC = [6]byte{0x5a, 0x94, 0xef, 0xe4, 0x0c, 0xee}

// setupNetwork configures the network backend.
func (v *VM) setupNetwork() error {
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

func (v *VM) setupGVisor() error {
	logrus.Info("configuring gvisor-tap-vsock network")

	addr, err := network.ParseUnixAddr(v.cfg.GVPVNetAddr)
	if err != nil {
		return err
	}

	path := cstr(addr.Path)
	defer free(path)

	var mac [6]C.uint8_t
	for i, b := range guestMAC {
		mac[i] = C.uint8_t(b)
	}

	var ret C.int32_t
	if runtime.GOOS == "linux" {
		ret = C.krun_add_net_unixstream(
			C.uint32_t(v.ctxID),
			path,
			-1,
			&mac[0],
			C.COMPAT_NET_FEATURES,
			0,
		)
	} else {
		ret = C.krun_add_net_unixgram(
			C.uint32_t(v.ctxID),
			path,
			-1,
			&mac[0],
			C.COMPAT_NET_FEATURES,
			C.NET_FLAG_VFKIT,
		)
	}

	if ret != 0 {
		return errCode(ret)
	}
	return nil
}
