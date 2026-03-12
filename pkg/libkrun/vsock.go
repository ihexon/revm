//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <libkrun.h>
*/
import "C"

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"runtime"

	"github.com/sirupsen/logrus"
)

// setupVSock configures the VSock device and port mappings.
func (v *VM) setupVSock() error {
	must(C.krun_disable_implicit_vsock(C.uint32_t(v.ctxID)))

	var features C.uint32_t
	if v.cfg.VirtualNetworkMode == define.TSI {
		features = C.KRUN_TSI_HIJACK_INET
		if runtime.GOOS == "darwin" {
			// macOS doesn't support KRUN_TSI_HIJACK_UNIX
			features = C.KRUN_TSI_HIJACK_INET
		}
	}

	must(C.krun_add_vsock(C.uint32_t(v.ctxID), features))

	// Map ignition server port
	addr, err := network.ParseUnixAddr(v.cfg.IgnitionServerCfg.ListenSockAddr)
	if err != nil {
		return err
	}

	path := cstr(addr.Path)
	defer free(path)

	ret := C.krun_add_vsock_port2(
		C.uint32_t(v.ctxID),
		C.uint32_t(define.DefaultVSockPort),
		path,
		false,
	)
	if ret != 0 {
		return errCode(ret)
	}

	logrus.Infof("vsock port %d → %s", define.DefaultVSockPort, addr.Path)
	return nil
}
