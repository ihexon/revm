//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func GetAvailablePort(preferredPort uint16) (uint64, error) {
	// 使用 127.0.0.1 检查，因为实际使用时绑定的是 localhost
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("127.0.0.1:%d", preferredPort))
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp4", addr)
	if err == nil {
		_ = l.Close()
		return uint64(preferredPort), nil
	}

	// Fallback to ephemeral port
	addr, err = net.ResolveTCPAddr("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err = net.ListenTCP("tcp4", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()

	return uint64(l.Addr().(*net.TCPAddr).Port), nil
}

type Addr struct {
	Scheme string
	Host   string // hostname or IP (no brackets)
	Port   int
	Path   string
}

func ParseUnixAddr(raw string) (*Addr, error) {
	if !strings.Contains(raw, "unix://") && !strings.Contains(raw, "unixgram://") {
		return nil, fmt.Errorf("scheme missing, expected format: unix://")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("missing scheme")
	}
	if u.Path == "" {
		return nil, fmt.Errorf("missing path")
	}

	return &Addr{
		Scheme: u.Scheme,
		Path:   u.Path,
	}, nil
}

func ParseTcpAddr(raw string) (*Addr, error) {
	if !strings.Contains(raw, "tcp://") {
		return nil, fmt.Errorf("scheme missing, expected format: tcp://<host>:<port>")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("missing scheme")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host:port")
	}

	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("split host/port: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	return &Addr{
		Scheme: u.Scheme,
		Host:   host, // IPv6 will be un-bracketed here
		Port:   port,
	}, nil
}
