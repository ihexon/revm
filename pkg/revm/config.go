//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
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
	Remove    bool `json:"remove,omitempty"`
	paths     *sessionRun
	RunMode   RunMode `json:"runMode,omitempty"`
	SessionID string  `json:"sessionID,omitempty"` // session name
	CPUs      int     `json:"cpus,omitempty"`      // 0 → host CPU count
	MemoryMB  uint64  `json:"memoryMB,omitempty"`  // 0 → host total RAM

	Exec        execution.Request `json:"exec"`
	PTY         bool              `json:"pty,omitempty"`
	TTY         string            `json:"tty,omitempty"`
	StopTimeout time.Duration     `json:"stopTimeout"`

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
		Network:     "gvisor",
		LogLevel:    "info",
		Exec:        execution.Request{WorkDir: "/"},
		TTY:         "auto",
		StopTimeout: 10 * time.Second,
	}
}

// --- Normalization & Validation --------------------------------------------

// NormalizeConfig returns a copy of cfg with defaults resolved.
func NormalizeConfig(cfg Config) (Config, error) {
	if cfg.TTY == "" {
		cfg.TTY = "auto"
	}
	if cfg.Network == "" {
		cfg.Network = "gvisor"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.Exec.WorkDir == "" {
		cfg.Exec.WorkDir = "/"
	}

	if cfg.RunMode != ModeAttach && cfg.RunMode != ModeControl {
		if cfg.CPUs == 0 {
			cfg.CPUs = min(runtime.NumCPU(), 4)
		}

		if cfg.MemoryMB == 0 {
			m, err := mem.VirtualMemory()
			if err != nil {
				return Config{}, fmt.Errorf("detect host memory: %w", err)
			}
			cfg.MemoryMB = max(512, min(2048, m.Available/1024/1024/4))
		}
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	if cfg.Proxy {
		proxyEnv, err := systemProxyEnv(define.VNetMode(cfg.Network))
		if err != nil {
			return Config{}, err
		}
		// Explicit --env values take precedence over detected defaults.
		cfg.Exec.Env = append(proxyEnv, cfg.Exec.Env...)
	}
	return cfg, nil
}

func validateConfig(cfg Config) error {
	if cfg.StopTimeout < 0 {
		return fmt.Errorf("--stop-timeout must not be negative")
	}
	if cfg.TTY != "auto" && cfg.TTY != "always" && cfg.TTY != "never" {
		return fmt.Errorf("--tty must be auto, always or never")
	}
	if _, err := logrus.ParseLevel(cfg.LogLevel); err != nil {
		return fmt.Errorf("invalid --log-level %q", cfg.LogLevel)
	}
	for _, env := range cfg.Exec.Env {
		key, _, ok := strings.Cut(env, "=")
		if !ok || key == "" || strings.ContainsAny(env, "\x00") {
			return fmt.Errorf("invalid environment variable %q: expected KEY=VALUE", env)
		}
	}
	if !path.IsAbs(cfg.Exec.WorkDir) {
		return fmt.Errorf("--workdir must be an absolute guest path")
	}
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
	if strings.HasPrefix(id, ".") {
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
	if len(cfg.PortUnforwards) > 0 && len(cfg.Exec.Argv) > 0 {
		return fmt.Errorf("port unexport cannot be combined with an attach command")
	}
	if len(cfg.PortForwards) > 0 && len(cfg.Exec.Argv) > 0 {
		return fmt.Errorf("port export cannot be combined with an attach command")
	}
	return nil
}

func validateControlConfig(cfg Config) error {
	if len(cfg.Exec.Argv) > 0 {
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
		if len(cfg.Exec.Argv) == 0 || cfg.Exec.Argv[0] == "" {
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
	if cfg.RunMode == ModeContainer && cfg.Network != string(define.GVISOR) {
		return fmt.Errorf("container mode requires gvisor network")
	}
	if len(cfg.PortForwards) > 0 && cfg.Network != string(define.GVISOR) {
		return fmt.Errorf("port export requires network %q, got %q", define.GVISOR, cfg.Network)
	}

	return nil
}

// ValidateSessionID checks a logical session name without creating resources.
func ValidateSessionID(id string) error { return validateSessionID(id) }
