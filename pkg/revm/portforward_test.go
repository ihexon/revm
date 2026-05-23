//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"reflect"
	"testing"
)

func TestParsePortExportSpec(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want define.PortForward
	}{
		{
			name: "host and guest ports",
			spec: "8080:80",
			want: define.PortForward{
				Protocol:  "tcp",
				HostIP:    define.LocalHost,
				HostPort:  8080,
				GuestIP:   define.GuestIP,
				GuestPort: 80,
			},
		},
		{
			name: "explicit host IP",
			spec: "0.0.0.0:8080:80",
			want: define.PortForward{
				Protocol:  "tcp",
				HostIP:    "0.0.0.0",
				HostPort:  8080,
				GuestIP:   define.GuestIP,
				GuestPort: 80,
			},
		},
		{
			name: "explicit protocol",
			spec: "tcp:127.0.0.1:8443:443",
			want: define.PortForward{
				Protocol:  "tcp",
				HostIP:    define.LocalHost,
				HostPort:  8443,
				GuestIP:   define.GuestIP,
				GuestPort: 443,
			},
		},
		{
			name: "slash protocol",
			spec: "tcp/8080:80",
			want: define.PortForward{
				Protocol:  "tcp",
				HostIP:    define.LocalHost,
				HostPort:  8080,
				GuestIP:   define.GuestIP,
				GuestPort: 80,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePortExportSpec(tt.spec)
			if err != nil {
				t.Fatalf("ParsePortExportSpec() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParsePortExportSpec() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParsePortUnexportSpec(t *testing.T) {
	got, err := ParsePortUnexportSpec("127.0.0.1:8080")
	if err != nil {
		t.Fatalf("ParsePortUnexportSpec() error = %v", err)
	}
	want := define.PortForward{
		Protocol: "tcp",
		HostIP:   define.LocalHost,
		HostPort: 8080,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePortUnexportSpec() = %#v, want %#v", got, want)
	}
}

func TestParsePortForwardSpecRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"",
		"udp:8080:80",
		"8080",
		"127.0.0.1:8080:80:1",
		"localhost:8080:80",
		"[::1]:8080:80",
		"0:80",
		"8080:0",
	}

	for _, spec := range tests {
		t.Run(spec, func(t *testing.T) {
			if _, err := ParsePortExportSpec(spec); err == nil {
				t.Fatalf("ParsePortExportSpec(%q) succeeded, want error", spec)
			}
		})
	}
}

func TestValidatePortForwardSetRejectsReservedAndDuplicatePorts(t *testing.T) {
	forward := define.PortForward{
		Protocol:  "tcp",
		HostIP:    define.LocalHost,
		HostPort:  8080,
		GuestIP:   define.GuestIP,
		GuestPort: 80,
	}

	if err := validatePortForwardSet([]define.PortForward{forward}, "127.0.0.1:8080"); err == nil {
		t.Fatal("validatePortForwardSet() accepted reserved SSH forward")
	}
	if err := validatePortForwardSet([]define.PortForward{forward, forward}, "127.0.0.1:6123"); err == nil {
		t.Fatal("validatePortForwardSet() accepted duplicate forwards")
	}
}

func TestValidatePortForwardSetRejectsMalformedStructuredForwards(t *testing.T) {
	tests := []struct {
		name    string
		forward define.PortForward
	}{
		{
			name: "empty protocol",
			forward: define.PortForward{
				HostIP:    define.LocalHost,
				HostPort:  8080,
				GuestIP:   define.GuestIP,
				GuestPort: 80,
			},
		},
		{
			name: "ipv6 host",
			forward: define.PortForward{
				Protocol:  "tcp",
				HostIP:    "::1",
				HostPort:  8080,
				GuestIP:   define.GuestIP,
				GuestPort: 80,
			},
		},
		{
			name: "zero guest port",
			forward: define.PortForward{
				Protocol: "tcp",
				HostIP:   define.LocalHost,
				HostPort: 8080,
				GuestIP:  define.GuestIP,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePortForwardSet([]define.PortForward{tt.forward}, "127.0.0.1:6123"); err == nil {
				t.Fatal("validatePortForwardSet() accepted malformed structured forward")
			}
		})
	}
}
