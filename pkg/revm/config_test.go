//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"runtime"
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

func TestNormalizeConfigDefaultsGPUOff(t *testing.T) {
	cfg := DefaultConfig().WithSessionID("gpu-default").WithCommand("true")
	cfg.RunMode = ModeRootfs

	got, err := NormalizeConfig(*cfg)
	if err != nil {
		t.Fatalf("NormalizeConfig() error = %v", err)
	}
	if got.GPU != define.GPUOff {
		t.Fatalf("GPU = %q, want %q", got.GPU, define.GPUOff)
	}
}

func TestNormalizeConfigRejectsUnknownGPU(t *testing.T) {
	cfg := DefaultConfig().WithSessionID("gpu-invalid").WithCommand("true").WithGPU("unknown")
	cfg.RunMode = ModeRootfs

	if _, err := NormalizeConfig(*cfg); err == nil {
		t.Fatal("NormalizeConfig() accepted an unknown GPU backend")
	}
}

func TestNormalizeConfigVenusPlatformGate(t *testing.T) {
	cfg := DefaultConfig().WithSessionID("gpu-venus").WithCommand("true").WithGPU(define.GPUVenus)
	cfg.RunMode = ModeRootfs

	_, err := NormalizeConfig(*cfg)
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		if err != nil {
			t.Fatalf("NormalizeConfig() rejected Venus on macOS arm64: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("NormalizeConfig() accepted Venus on an unsupported platform")
	}
}
