//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

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

type Addr struct {
	Scheme string
	Host   string // hostname or IP (no brackets)
	Port   int
	Path   string
}

func ParseUnixAddr(raw string) (*Addr, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "unix" && u.Scheme != "unixgram" {
		return nil, fmt.Errorf("invalid scheme %q, expected unix:// or unixgram://", u.Scheme)
	}
	if u.Host != "" {
		return nil, fmt.Errorf("unix socket address must not contain a host")
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
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "tcp" {
		return nil, fmt.Errorf("invalid scheme %q, expected tcp://<host>:<port>", u.Scheme)
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
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port %d is outside range 1-65535", port)
	}

	return &Addr{
		Scheme: u.Scheme,
		Host:   host, // IPv6 will be un-bracketed here
		Port:   port,
	}, nil
}
