//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"bytes"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"
	"testing"
)

func TestNewCtlConfigUsesControl(t *testing.T) {
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

func TestNewAttachConfig(t *testing.T) {
	cfg := NewAttachConfig(AttachOptions{
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

func TestValidateCtlOptionsRequiresPortUpdate(t *testing.T) {
	if err := validateCtlOptions(CtlOptions{}); err == nil {
		t.Fatal("validateCtlOptions() accepted ctl without an operation")
	}
}

func TestNewCtlConfigUsesPortList(t *testing.T) {
	cfg := NewCtlConfig(CtlOptions{
		Logging:   LoggingOptions{Level: "info"},
		SessionID: "myengine",
		ListPort:  true,
	})

	if cfg.RunMode != revm.ModeControl {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeControl)
	}
	if !cfg.PortList {
		t.Fatal("PortList = false, want true")
	}
	if len(cfg.PortForwards) != 0 || len(cfg.PortUnforwards) != 0 {
		t.Fatalf("port updates = (%#v, %#v), want empty", cfg.PortForwards, cfg.PortUnforwards)
	}
}

func TestNewCtlConfigUsesRootfsExport(t *testing.T) {
	cfg := NewCtlConfig(CtlOptions{
		Logging:      LoggingOptions{Level: "info"},
		SessionID:    "myengine",
		ExportRootfs: "/tmp/rootfs.tar.zst",
	})

	if cfg.RunMode != revm.ModeControl {
		t.Fatalf("RunMode = %q, want %q", cfg.RunMode, revm.ModeControl)
	}
	if cfg.RootfsExport != "/tmp/rootfs.tar.zst" {
		t.Fatalf("RootfsExport = %q, want /tmp/rootfs.tar.zst", cfg.RootfsExport)
	}
	if cfg.PortList || len(cfg.PortForwards) != 0 || len(cfg.PortUnforwards) != 0 {
		t.Fatalf("other control operations enabled: list=%v forwards=%#v unforwards=%#v", cfg.PortList, cfg.PortForwards, cfg.PortUnforwards)
	}
}

func TestValidateCtlOptionsRejectsListWithPortUpdates(t *testing.T) {
	err := validateCtlOptions(CtlOptions{
		ListPort: true,
		PortUpdates: PortUpdates{
			Exports: []define.PortForward{{
				Protocol:  "tcp",
				HostIP:    "127.0.0.1",
				HostPort:  8888,
				GuestPort: 8888,
			}},
		},
	})
	if err == nil {
		t.Fatal("validateCtlOptions() accepted --list-port with port updates")
	}
}

func TestValidateCtlOptionsRejectsRootfsExportWithPortUpdates(t *testing.T) {
	err := validateCtlOptions(CtlOptions{
		ExportRootfs: "/tmp/rootfs.tar.zst",
		PortUpdates: PortUpdates{
			Exports: []define.PortForward{{
				Protocol:  "tcp",
				HostIP:    "127.0.0.1",
				HostPort:  8888,
				GuestPort: 8888,
			}},
		},
	})
	if err == nil {
		t.Fatal("validateCtlOptions() accepted --export-rootfs with port updates")
	}
}

func TestValidateCtlOptionsRejectsCommandArguments(t *testing.T) {
	err := validateCtlOptions(CtlOptions{
		ListPort: true,
		Command:  []string{"sh"},
	})
	if err == nil {
		t.Fatal("validateCtlOptions() accepted command arguments")
	}
}

func TestWritePortMappings(t *testing.T) {
	var buf bytes.Buffer
	err := writePortMappings(&buf, []define.PortMapping{
		{Protocol: "tcp", Local: "127.0.0.1:8080", Remote: "192.168.127.2:80"},
		{Protocol: "tcp", Local: "127.0.0.1:6123", Remote: "192.168.127.2:22"},
	})
	if err != nil {
		t.Fatalf("writePortMappings() error = %v", err)
	}

	want := "PROTOCOL  HOST            GUEST\n" +
		"tcp       127.0.0.1:6123  192.168.127.2:22\n" +
		"tcp       127.0.0.1:8080  192.168.127.2:80\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
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
