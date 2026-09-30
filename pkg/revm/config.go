//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

// RunMode selects the VM run mode.
type RunMode string

const (
	// ModeRootfs boots the VM with a rootfs and executes a command.
	ModeRootfs RunMode = "rootfs"
	// ModeContainer boots the VM with the built-in container runtime (Podman).
	ModeContainer RunMode = "container"
	// ModeAttach connects to an existing VM session without building a VM.
	ModeAttach RunMode = "attach"
	// ModeControl performs control-plane operations against an existing VM.
	ModeControl RunMode = "control"
)

func (m RunMode) IsValid() bool {
	switch m {
	case ModeRootfs, ModeContainer, ModeAttach, ModeControl:
		return true
	default:
		return false
	}
}

type Config struct {
	RunMode   RunMode `json:"runMode,omitempty"`
	SessionID string  `json:"sessionID,omitempty"` // session name
	CPUs      int     `json:"cpus,omitempty"`      // 0 → host CPU count
	MemoryMB  uint64  `json:"memoryMB,omitempty"`  // 0 → host total RAM

	// Command specifies the program to run inside the VM.
	// It is required by run mode and optional in attach mode.
	Command []string `json:"command,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	PTY     bool     `json:"pty,omitempty"`

	Network              string               `json:"network,omitempty"` // "gvisor" | "tsi"
	Mounts               []string             `json:"mounts,omitempty"`  // "/host:/guest[,ro]"
	Disks                []RawDiskSpec        `json:"disks,omitempty"`
	ContainerDisk        *ContainerDiskSpec   `json:"containerDisk,omitempty"`
	PodmanProxyAPIFile   string               `json:"podmanProxyAPIFile,omitempty"`
	ManageAPIFile        string               `json:"manageAPIFile,omitempty"`
	SSHKeyFileSymbolPath string               `json:"SSHKeyFileSymbolPath,omitempty"`
	ReportURL            string               `json:"reportURL,omitempty"`
	Proxy                bool                 `json:"proxy,omitempty"`
	LogLevel             string               `json:"logLevel,omitempty"` // default "info"
	PortList             bool                 `json:"portList,omitempty"`
	PortForwards         []define.PortForward `json:"portForwards,omitempty"`
	PortUnforwards       []define.PortForward `json:"portUnforwards,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults pre-filled.
// Zero-value resource fields (CPUs, MemoryMB) are resolved at VM creation time.
// Session identity must be supplied explicitly by the caller.
func DefaultConfig() *Config {
	return &Config{
		Network:  "gvisor",
		LogLevel: "info",
		WorkDir:  "/",
	}
}

// --- Normalization & Validation --------------------------------------------

// NormalizeConfig returns a copy of cfg with defaults resolved.
func NormalizeConfig(cfg Config) (Config, error) {
	if cfg.Network == "" {
		cfg.Network = "gvisor"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = "/"
	}

	if cfg.RunMode != ModeAttach && cfg.RunMode != ModeControl {
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
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateConfig(cfg Config) error {
	if err := validateSessionID(cfg.SessionID); err != nil {
		return err
	}

	if cfg.SessionID == "" {
		return fmt.Errorf("session name must not be empty, flag --id is required")
	}

	if !cfg.RunMode.IsValid() {
		return fmt.Errorf("invalid run mode %q", cfg.RunMode)
	}

	switch cfg.RunMode {
	case ModeAttach:
		return validateAttachConfig(cfg)
	case ModeControl:
		return validateControlConfig(cfg)
	case ModeRootfs, ModeContainer:
		return validateBuildConfig(cfg)
	default:
		return fmt.Errorf("invalid run mode %q", cfg.RunMode)
	}
}

// validateSessionID keeps the session directory below ~/.cache/revm. Session
// IDs are also used in socket and lock file names, so accepting path syntax
// here would let a caller escape the managed workspace.
func validateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session name must not be empty, flag --id is required")
	}
	if id == "." || id == ".." {
		return fmt.Errorf("session name %q is reserved", id)
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("session name must not contain leading or trailing whitespace")
	}
	if len(id) > 64 {
		return fmt.Errorf("session name must be at most 64 characters")
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			continue
		}
		return fmt.Errorf("session name %q contains invalid character %q; use letters, numbers, '.', '_' or '-'", id, c)
	}
	return nil
}

func validateAttachConfig(cfg Config) error {
	if len(cfg.PortUnforwards) > 0 && len(cfg.Command) > 0 {
		return fmt.Errorf("port unexport cannot be combined with an attach command")
	}
	if len(cfg.PortForwards) > 0 && len(cfg.Command) > 0 {
		return fmt.Errorf("port export cannot be combined with an attach command")
	}
	return nil
}

func validateControlConfig(cfg Config) error {
	if len(cfg.Command) > 0 {
		return fmt.Errorf("control operations cannot be combined with an attach command")
	}
	hasPortUpdates := len(cfg.PortForwards) > 0 || len(cfg.PortUnforwards) > 0
	operationCount := 0
	if cfg.PortList {
		operationCount++
	}
	if hasPortUpdates {
		operationCount++
	}
	if operationCount > 1 {
		return fmt.Errorf("control operations cannot be combined")
	}
	if operationCount == 0 {
		return fmt.Errorf("control mode requires a control operation")
	}
	return nil
}

func validateBuildConfig(cfg Config) error {
	if len(cfg.PortUnforwards) > 0 {
		return fmt.Errorf("port unexport requires attach mode")
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
	if cfg.CPUs > 32 {
		return fmt.Errorf("cpus must be at most 32 (libkrun supported limit), got %d", cfg.CPUs)
	}

	switch cfg.Network {
	case "gvisor", "tsi":
		// ok
	default:
		return fmt.Errorf("network must be \"gvisor\" or \"tsi\", got %q", cfg.Network)
	}
	if len(cfg.PortForwards) > 0 && cfg.Network != string(define.GVISOR) {
		return fmt.Errorf("port export requires network %q, got %q", define.GVISOR, cfg.Network)
	}

	return nil
}
