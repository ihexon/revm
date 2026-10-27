package revm

import (
	"linuxvm/pkg/define"
	"testing"
)

func TestRawDiskSyncNoneSurvivesPreparation(t *testing.T) {
	for _, tc := range []struct {
		option string
		want   uint32
	}{{"", define.SyncModeRelaxed}, {",sync=none", define.SyncModeNone}, {",sync=full", define.SyncModeFull}} {
		spec, err := ParseRawDiskSpec("/tmp/disk.ext4" + tc.option)
		if err != nil {
			t.Fatal(err)
		}
		dev := applyRawDiskRuntimeOptions(define.BlockDeviceSpec{}, spec)
		if dev.SyncMode != tc.want {
			t.Fatalf("%s: sync=%d want %d", tc.option, dev.SyncMode, tc.want)
		}
	}
}

func TestOmittedSessionIsDisposable(t *testing.T) {
	cfg := testConfig()
	cfg.SessionID = ""
	resolved, err := normalizeVMOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SessionID == "" || !resolved.Remove {
		t.Fatal("unnamed run must be disposable")
	}
	named, err := normalizeVMOptions(testConfig())
	if err != nil || named.Remove {
		t.Fatalf("named session must retain records: %v", err)
	}
}

func TestNetworkModes(t *testing.T) {
	for _, mode := range []string{"", define.NetworkGVisor, define.NetworkTSI} {
		cfg := testConfig()
		cfg.Network = mode
		got, err := normalizeVMOptions(cfg)
		if err != nil {
			t.Fatal(err)
		}
		want := mode
		if want == "" {
			want = define.NetworkGVisor
		}
		if got.Network != want {
			t.Fatalf("network=%q, want %q", got.Network, want)
		}
	}
	cfg := testConfig()
	cfg.Network = "invalid"
	if _, err := normalizeVMOptions(cfg); err == nil {
		t.Fatal("accepted unknown network")
	}
	cfg.Network = define.NetworkTSI
	cfg.PortForwards, _ = ParsePortExportSpecs([]string{"8080:80"})
	if _, err := normalizeVMOptions(cfg); err == nil {
		t.Fatal("accepted gvproxy forwards with TSI")
	}
	cfg.PortForwards = nil
	cfg.RunMode, cfg.VolumeName, cfg.Exec.Argv = define.ContainerMode, "data", nil
	if _, err := normalizeVMOptions(cfg); err == nil {
		t.Fatal("accepted Podman with TSI")
	}
}
