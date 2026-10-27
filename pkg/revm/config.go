//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"path"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
)

type ExecOptions struct {
	Exec execution.Request `json:"exec"`
	TTY  string            `json:"tty,omitempty"`
}

type VMOptions struct {
	ExecOptions
	Remove             bool                  `json:"remove,omitempty"`
	VolumeName         string                `json:"volumeName,omitempty"`
	RunMode            define.RunMode        `json:"runMode,omitempty"`
	SessionID          string                `json:"sessionID,omitempty"`
	CPUs               int                   `json:"cpus,omitempty"`
	MemoryMB           uint64                `json:"memoryMB,omitempty"`
	Network            string                `json:"network,omitempty"`
	Mounts             []define.VirtioFSSpec `json:"mounts,omitempty"`
	Disks              []RawDiskSpec         `json:"disks,omitempty"`
	ContainerDisk      *ContainerDiskSpec    `json:"containerDisk,omitempty"`
	PodmanProxyAPIFile string                `json:"podmanProxyAPIFile,omitempty"`
	ManageAPIFile      string                `json:"manageAPIFile,omitempty"`
	Proxy              bool                  `json:"proxy,omitempty"`
	LogLevel           string                `json:"logLevel,omitempty"`
	PortForwards       []define.PortForward  `json:"portForwards,omitempty"`
}

func normalizeExecOptions(opts *ExecOptions) error {
	if opts.TTY == "" {
		opts.TTY = "auto"
	}
	if opts.Exec.WorkDir == "" {
		opts.Exec.WorkDir = "/"
	}
	if opts.TTY != "auto" && opts.TTY != "always" && opts.TTY != "never" {
		return fmt.Errorf("--tty must be auto, always or never")
	}
	if !path.IsAbs(opts.Exec.WorkDir) {
		return fmt.Errorf("--workdir must be an absolute guest path")
	}
	for _, env := range opts.Exec.Env {
		key, _, ok := strings.Cut(env, "=")
		if !ok || key == "" || strings.ContainsAny(env, "\x00") {
			return fmt.Errorf("invalid environment variable %q: expected KEY=VALUE", env)
		}
	}
	return nil
}

func normalizeVMOptions(cfg VMOptions) (VMOptions, error) {
	if err := normalizeExecOptions(&cfg.ExecOptions); err != nil {
		return VMOptions{}, err
	}
	if cfg.Network == "" {
		cfg.Network = "gvisor"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.SessionID == "" {
		cfg.SessionID, cfg.Remove = "run-"+uuid.NewString(), true
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = min(runtime.NumCPU(), 4)
	}
	if cfg.MemoryMB == 0 {
		m, err := mem.VirtualMemory()
		if err != nil {
			return VMOptions{}, fmt.Errorf("detect host memory: %w", err)
		}
		cfg.MemoryMB = max(512, min(2048, m.Available/1024/1024/4))
	}
	if err := validateVMOptions(cfg); err != nil {
		return VMOptions{}, err
	}
	if cfg.Proxy {
		proxyEnv, err := systemProxyEnv(define.VNetMode(cfg.Network))
		if err != nil {
			return VMOptions{}, err
		}
		cfg.Exec.Env = append(proxyEnv, cfg.Exec.Env...)
	}
	return cfg, nil
}

func validateVMOptions(cfg VMOptions) error {
	if err := validateSessionID(cfg.SessionID); err != nil {
		return err
	}
	if _, err := logrus.ParseLevel(cfg.LogLevel); err != nil {
		return fmt.Errorf("invalid --log-level %q", cfg.LogLevel)
	}
	switch cfg.RunMode {
	case define.ContainerMode:
		if (cfg.VolumeName == "") == (cfg.ContainerDisk == nil) {
			return fmt.Errorf("podman requires exactly one of --volume NAME or --container-disk PATH")
		}
		if len(cfg.Exec.Argv) != 0 {
			return fmt.Errorf("podman does not accept a command; use revm exec")
		}
	case define.RootFsMode:
		if cfg.VolumeName != "" || cfg.ContainerDisk != nil {
			return fmt.Errorf("container storage requires podman mode")
		}
		if len(cfg.Exec.Argv) == 0 || cfg.Exec.Argv[0] == "" {
			return fmt.Errorf("rootfs mode requires a non-empty command")
		}
	default:
		return fmt.Errorf("invalid run mode %q", cfg.RunMode)
	}
	if cfg.MemoryMB < 512 {
		return fmt.Errorf("memory must be at least 512 MB, got %d", cfg.MemoryMB)
	}
	if cfg.CPUs < 1 || cfg.CPUs > 32 {
		return fmt.Errorf("cpus must be between 1 and 32, got %d", cfg.CPUs)
	}
	if cfg.Network != "gvisor" && cfg.Network != "tsi" {
		return fmt.Errorf("network must be gvisor or tsi, got %q", cfg.Network)
	}
	if cfg.RunMode == define.ContainerMode && cfg.Network != "gvisor" {
		return fmt.Errorf("container mode requires gvisor network")
	}
	if len(cfg.PortForwards) > 0 && cfg.Network != "gvisor" {
		return fmt.Errorf("port export requires gvisor network")
	}
	return validatePortForwardSet(cfg.PortForwards)
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

// ValidateSessionID checks a logical session name without creating resources.
func ValidateSessionID(id string) error { return validateSessionID(id) }
