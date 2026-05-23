//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"
	"testing"
)

func TestNewRunPlan(t *testing.T) {
	portUpdate := PortUpdates{
		Exports: []define.PortForward{{
			Protocol:  "tcp",
			HostIP:    "127.0.0.1",
			HostPort:  8888,
			GuestPort: 8888,
		}},
	}

	tests := []struct {
		name        string
		attach      bool
		ports       PortUpdates
		defaultMode revm.RunMode
		wantMode    revm.RunMode
	}{
		{
			name:        "port updates take precedence over boot",
			ports:       portUpdate,
			defaultMode: revm.ModeRootfs,
			wantMode:    revm.ModeControl,
		},
		{
			name:        "port updates take precedence over attach",
			attach:      true,
			ports:       portUpdate,
			defaultMode: revm.ModeRootfs,
			wantMode:    revm.ModeControl,
		},
		{
			name:        "attach without port updates",
			attach:      true,
			defaultMode: revm.ModeRootfs,
			wantMode:    revm.ModeAttach,
		},
		{
			name:        "boot without port updates",
			defaultMode: revm.ModeContainer,
			wantMode:    revm.ModeContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRunPlan(tt.attach, tt.ports, tt.defaultMode)
			if got.Mode != tt.wantMode {
				t.Fatalf("NewRunPlan() mode = %q, want %q", got.Mode, tt.wantMode)
			}
		})
	}
}
