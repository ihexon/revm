//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"testing"
)

func TestNormalizeConfigControlDoesNotResolveBootResources(t *testing.T) {
	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithControl([]define.PortForward{{
			Protocol:  "tcp",
			HostIP:    "127.0.0.1",
			HostPort:  8888,
			GuestPort: 8888,
		}}, nil)

	got, err := NormalizeConfig(*cfg)
	if err != nil {
		t.Fatalf("NormalizeConfig() error = %v", err)
	}
	if got.RunMode != ModeControl {
		t.Fatalf("RunMode = %q, want %q", got.RunMode, ModeControl)
	}
	if got.CPUs != 0 {
		t.Fatalf("CPUs = %d, want 0 for control mode", got.CPUs)
	}
	if got.MemoryMB != 0 {
		t.Fatalf("MemoryMB = %d, want 0 for control mode", got.MemoryMB)
	}
}
