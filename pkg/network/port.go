package network

import (
	"net"
)

func GetAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close() //nolint:errcheck

	return l.Addr().(*net.TCPAddr).Port, nil
}
