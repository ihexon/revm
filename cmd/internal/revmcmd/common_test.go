//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"
	"testing"
)

func TestNewCtlConfigUsesControlForPortUpdates(t *testing.T) {
	cfg := NewCtlConfig(CtlOptions{
		Logging:   LoggingOptions{Level: "info"},
		SessionID: "myengine",
		PortUpdates: PortUpdates{
			Exports: []define.PortForward{{
				Protocol:  "tcp",
				HostIP:    "127.0.0.1",
				HostPort:  8888,
				GuestPort: 8888,
			}},
		},
		Command: []string{"sh"},
	})

	if cfg.RunMode != revm.ModeControl {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeControl)
	}
	if len(cfg.Command) != 0 {
		t.Fatalf("Command = %#v, want empty for control mode", cfg.Command)
	}
	if len(cfg.PortForwards) != 1 {
		t.Fatalf("PortForwards len = %d, want 1", len(cfg.PortForwards))
	}
}

func TestNewCtlConfigUsesAttachWithoutPortUpdates(t *testing.T) {
	cfg := NewCtlConfig(CtlOptions{
		Logging:   LoggingOptions{Level: "info"},
		SessionID: "myengine",
		PTY:       true,
		Command:   []string{"sh"},
	})

	if cfg.RunMode != revm.ModeAttach {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeAttach)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "sh" {
		t.Fatalf("Command = %#v, want [sh]", cfg.Command)
	}
	if !cfg.PTY {
		t.Fatal("PTY = false, want true")
	}
}

func TestNewRunConfig(t *testing.T) {
	cfg := NewRunConfig(RunOptions{
		Logging:   LoggingOptions{Level: "info"},
		SessionID: "myengine",
		Command:   []string{"sh"},
	})

	if cfg.RunMode != revm.ModeRootfs {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeRootfs)
	}
	if len(cfg.Command) != 1 || cfg.Command[0] != "sh" {
		t.Fatalf("Command = %#v, want [sh]", cfg.Command)
	}
}

func TestNewDockerdConfig(t *testing.T) {
	cfg := NewDockerdConfig(DockerdOptions{
		Logging:   LoggingOptions{Level: "info"},
		SessionID: "myengine",
	})

	if cfg.RunMode != revm.ModeContainer {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeContainer)
	}
	if cfg.Network != string(define.GVISOR) {
		t.Fatalf("Network = %q, want %q", cfg.Network, define.GVISOR)
	}
}
