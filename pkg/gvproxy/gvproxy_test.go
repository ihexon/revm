package gvproxy

import "testing"

func TestNewConfigUsesSpecWithoutMutation(t *testing.T) {
	spec := Spec{
		ControlAddr:         "unix:///tmp/revm/gvproxy.sock",
		NetAddr:             "unixgram:///tmp/revm/vnet.sock",
		NotifyAddr:          "unix:///tmp/revm/notify.sock",
		HostSSHForwardAddr:  "127.0.0.1:6123",
		GuestSSHListenAddr:  "0.0.0.0:2222",
		GuestIP:             "192.168.127.2",
		HostLoopbackAddress: "127.0.0.1",
	}

	cfg, err := NewConfig(spec)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ControlAddr != spec.ControlAddr {
		t.Fatalf("control addr = %q, want %q", cfg.ControlAddr, spec.ControlAddr)
	}
	if got := cfg.Stack.Forwards[spec.HostSSHForwardAddr]; got != "192.168.127.2:2222" {
		t.Fatalf("ssh forward = %q", got)
	}
	if got := cfg.Stack.NAT["192.168.127.254"]; got != spec.HostLoopbackAddress {
		t.Fatalf("host NAT = %q", got)
	}
	if got := cfg.Stack.DHCPStaticLeases[spec.GuestIP]; got == "" {
		t.Fatalf("missing DHCP static lease for guest IP")
	}
}

func TestNewConfigRejectsIncompleteSpec(t *testing.T) {
	if _, err := NewConfig(Spec{}); err == nil {
		t.Fatalf("expected empty spec to fail")
	}
}
