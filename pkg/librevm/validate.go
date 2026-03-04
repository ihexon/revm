//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"crypto/rand"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/mem"
)

// NormalizeConfig returns a copy of cfg with defaults resolved.
func NormalizeConfig(cfg Config) (Config, error) {
	if cfg.SessionID == "" {
		cfg.SessionID = randomName()
	}

	if cfg.CPUs <= 0 {
		cfg.CPUs = runtime.NumCPU()
	}

	if cfg.MemoryMB == 0 {
		m, err := mem.VirtualMemory()
		if err != nil {
			return Config{}, fmt.Errorf("detect host memory: %w", err)
		}
		cfg.MemoryMB = m.Total / 1024 / 1024
	}

	if cfg.Network == "" {
		cfg.Network = "gvisor"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = "/"
	}

	return cfg, nil
}

func validateConfig(cfg Config) error {
	if !cfg.RunMode.IsValid() {
		return fmt.Errorf(
			"mode must be %q, %q, %q or %q, got %q",
			ModeRootfs,
			ModeContainer,
			ModeOVMRun,
			ModeOVMInit,
			cfg.RunMode,
		)
	}

	if cfg.RunMode.IsOVM() {
		return fmt.Errorf("ovm mode %q is not yet implemented", cfg.RunMode)
	}

	if cfg.RunMode == ModeRootfs {
		if len(cfg.Command) == 0 || cfg.Command[0] == "" {
			return fmt.Errorf("rootfs mode requires a non-empty command")
		}
	}

	if cfg.MemoryMB < 512 {
		return fmt.Errorf("memory must be at least 512 MB, got %d", cfg.MemoryMB)
	}

	if cfg.CPUs < 1 {
		return fmt.Errorf("cpus must be at least 1, got %d", cfg.CPUs)
	}
	if cfg.CPUs > 255 {
		return fmt.Errorf("cpus must be at most 255 (libkrun uint8_t limit), got %d", cfg.CPUs)
	}

	switch cfg.Network {
	case "gvisor", "tsi":
		// ok
	default:
		return fmt.Errorf("network must be \"gvisor\" or \"tsi\", got %q", cfg.Network)
	}

	return nil
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomName() string {
	b := make([]byte, 8)
	randBytes := make([]byte, len(b))
	if _, err := rand.Read(randBytes); err != nil {
		for i := range b {
			b[i] = base62[i%len(base62)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = base62[int(randBytes[i])%len(base62)]
	}
	return string(b)
}
