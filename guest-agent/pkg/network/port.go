//go:build linux && (arm64 || amd64)

package network

import (
	"fmt"
	"net"
)

// GetAvailablePort returns an available TCP port on 127.0.0.1.
// If preferredPort is non-zero, it is returned if available; otherwise an
// ephemeral port assigned by the OS is returned.
func GetAvailablePort(preferredPort uint16) (uint64, error) {
	if preferredPort != 0 {
		addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("127.0.0.1:%d", preferredPort))
		if err != nil {
			return 0, err
		}
		l, err := net.ListenTCP("tcp4", addr)
		if err == nil {
			_ = l.Close()
			return uint64(preferredPort), nil
		}
	}

	// Fallback to ephemeral port
	addr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()

	return uint64(l.Addr().(*net.TCPAddr).Port), nil
}
