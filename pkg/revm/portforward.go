//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"net"
	"strconv"
	"strings"
)

const defaultPortForwardProtocol = "tcp"

func ParsePortExportSpecs(specs []string) ([]define.PortForward, error) {
	forwards := make([]define.PortForward, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec) == "" {
			continue
		}
		forward, err := ParsePortExportSpec(spec)
		if err != nil {
			return nil, err
		}
		forwards = append(forwards, forward)
	}
	return forwards, nil
}

func ParsePortExportSpec(spec string) (define.PortForward, error) {
	protocol, fields, err := splitPortSpec(spec)
	if err != nil {
		return define.PortForward{}, err
	}
	if len(fields) != 2 && len(fields) != 3 {
		return define.PortForward{}, fmt.Errorf("invalid port export %q: use HOST_PORT:GUEST_PORT or HOST_IP:HOST_PORT:GUEST_PORT", spec)
	}

	hostIP := define.LocalHost
	if len(fields) == 3 {
		hostIP = fields[0]
		fields = fields[1:]
	}

	hostPort, err := parseTCPPort(fields[0], "host port")
	if err != nil {
		return define.PortForward{}, fmt.Errorf("invalid port export %q: %w", spec, err)
	}
	guestPort, err := parseTCPPort(fields[1], "guest port")
	if err != nil {
		return define.PortForward{}, fmt.Errorf("invalid port export %q: %w", spec, err)
	}
	if err := validateListenIP(hostIP); err != nil {
		return define.PortForward{}, fmt.Errorf("invalid port export %q: %w", spec, err)
	}

	return define.PortForward{
		Protocol:  protocol,
		HostIP:    hostIP,
		HostPort:  hostPort,
		GuestIP:   define.GuestIP,
		GuestPort: guestPort,
	}, nil
}

func ParsePortUnexportSpecs(specs []string) ([]define.PortForward, error) {
	forwards := make([]define.PortForward, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec) == "" {
			continue
		}
		forward, err := ParsePortUnexportSpec(spec)
		if err != nil {
			return nil, err
		}
		forwards = append(forwards, forward)
	}
	return forwards, nil
}

func ParsePortUnexportSpec(spec string) (define.PortForward, error) {
	protocol, fields, err := splitPortSpec(spec)
	if err != nil {
		return define.PortForward{}, err
	}
	if len(fields) != 1 && len(fields) != 2 {
		return define.PortForward{}, fmt.Errorf("invalid port unexport %q: use HOST_PORT or HOST_IP:HOST_PORT", spec)
	}

	hostIP := define.LocalHost
	if len(fields) == 2 {
		hostIP = fields[0]
		fields = fields[1:]
	}

	hostPort, err := parseTCPPort(fields[0], "host port")
	if err != nil {
		return define.PortForward{}, fmt.Errorf("invalid port unexport %q: %w", spec, err)
	}
	if err := validateListenIP(hostIP); err != nil {
		return define.PortForward{}, fmt.Errorf("invalid port unexport %q: %w", spec, err)
	}

	return define.PortForward{
		Protocol: protocol,
		HostIP:   hostIP,
		HostPort: hostPort,
	}, nil
}

func splitPortSpec(spec string) (string, []string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", nil, fmt.Errorf("port spec must not be empty")
	}

	protocol := defaultPortForwardProtocol
	if before, after, ok := strings.Cut(spec, "/"); ok {
		protocol = strings.ToLower(strings.TrimSpace(before))
		spec = after
	} else if before, after, ok := strings.Cut(spec, ":"); ok && isPortForwardProtocol(before) {
		protocol = strings.ToLower(strings.TrimSpace(before))
		spec = after
	}
	if !isPortForwardProtocol(protocol) {
		return "", nil, fmt.Errorf("unsupported port forward protocol %q: only tcp is supported", protocol)
	}

	fields := strings.Split(spec, ":")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
		if fields[i] == "" {
			return "", nil, fmt.Errorf("port spec %q contains an empty field", spec)
		}
	}
	return protocol, fields, nil
}

func isPortForwardProtocol(value string) bool {
	return strings.EqualFold(value, defaultPortForwardProtocol)
}

func parseTCPPort(value, name string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%s %q must be a number between 1 and 65535", name, value)
	}
	if port == 0 {
		return 0, fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return uint16(port), nil
}

func validateListenIP(value string) error {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("host IP %q must be an IPv4 address", value)
	}
	return nil
}

func validateGuestIP(value string) error {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("guest IP %q must be an IPv4 address", value)
	}
	return nil
}

func portForwardLocal(forward define.PortForward) string {
	return net.JoinHostPort(forward.HostIP, strconv.Itoa(int(forward.HostPort)))
}

func portForwardRemote(forward define.PortForward) string {
	return net.JoinHostPort(forward.GuestIP, strconv.Itoa(int(forward.GuestPort)))
}

func validatePortForwardSet(forwards []define.PortForward, reservedLocal string) error {
	seen := map[string]struct{}{}
	for _, forward := range forwards {
		if err := validatePortForward(forward, true); err != nil {
			return err
		}
		local := portForwardLocal(forward)
		if local == reservedLocal {
			return fmt.Errorf("port export %s conflicts with internal SSH forward", local)
		}
		key := forward.Protocol + "/" + local
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate port export %s", local)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validatePortUpdateSet(forwards []define.PortForward, reservedLocal, action string) error {
	seen := map[string]struct{}{}
	for _, forward := range forwards {
		requireRemote := action == "export"
		if err := validatePortForward(forward, requireRemote); err != nil {
			return fmt.Errorf("port %s: %w", action, err)
		}
		local := portForwardLocal(forward)
		if local == reservedLocal {
			return fmt.Errorf("port %s %s conflicts with internal SSH forward", action, local)
		}
		key := forward.Protocol + "/" + local
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate port %s %s", action, local)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validatePortForward(forward define.PortForward, requireRemote bool) error {
	if !isPortForwardProtocol(forward.Protocol) {
		return fmt.Errorf("unsupported protocol %q: only tcp is supported", forward.Protocol)
	}
	if err := validateListenIP(forward.HostIP); err != nil {
		return err
	}
	if forward.HostPort == 0 {
		return fmt.Errorf("host port must be between 1 and 65535")
	}
	if !requireRemote {
		return nil
	}
	if err := validateGuestIP(forward.GuestIP); err != nil {
		return err
	}
	if forward.GuestPort == 0 {
		return fmt.Errorf("guest port must be between 1 and 65535")
	}
	return nil
}
