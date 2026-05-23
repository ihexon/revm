//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"runtime"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
)

// RunMode selects the VM run mode.
type RunMode string

const (
	// ModeRootfs boots the VM with a rootfs and executes a command.
	ModeRootfs RunMode = "rootfs"
	// ModeContainer boots the VM with the built-in container runtime (Podman).
	ModeContainer RunMode = "docker"
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
	Rootfs    string  `json:"rootfs,omitempty"`    // empty → built-in Alpine

	// Command specifies the program to run inside the VM.
	// It is required by rootfs mode and optional in attach mode.
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
	LogTo                string               `json:"logTo,omitempty"`
	PortForwards         []define.PortForward `json:"portForwards,omitempty"`
	PortUnforwards       []define.PortForward `json:"portUnforwards,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults pre-filled.
// Zero-value resource fields (CPUs, MemoryMB) are resolved at VM creation time.
// Session identity must be supplied explicitly with WithSessionID.
func DefaultConfig() *Config {
	return &Config{
		Network:  "gvisor",
		LogLevel: "info",
		WorkDir:  "/",
	}
}

// --- Chain (fluent) methods ------------------------------------------------

func (c *Config) WithMode(m RunMode) *Config {
	if m == "" {
		return c
	}
	c.RunMode = m
	return c
}

func (c *Config) WithSessionID(sessionID string) *Config {
	c.SessionID = sessionID
	return c
}

func (c *Config) WithAttach(cmdline ...string) *Config {
	c.RunMode = ModeAttach
	c.Command = nil
	return c.WithCommandLine(cmdline...)
}

func (c *Config) WithControl(portForwards, portUnforwards []define.PortForward) *Config {
	c.RunMode = ModeControl
	c.Command = nil
	c.PortForwards = append([]define.PortForward(nil), portForwards...)
	c.PortUnforwards = append([]define.PortForward(nil), portUnforwards...)
	return c
}

func (c *Config) WithCPUs(n int) *Config {
	if n <= 0 {
		c.CPUs = 0 // auto-detect in build vm
		return c
	}
	c.CPUs = n
	return c
}

func (c *Config) WithMemory(mb uint64) *Config {
	if mb == 0 {
		c.MemoryMB = 0 // auto-detect in build vm
		return c
	}
	c.MemoryMB = mb
	return c
}

func (c *Config) WithRootfs(path string) *Config {
	if path == "" {
		return c
	}
	c.Rootfs = path
	return c
}

func (c *Config) WithWorkDir(dir string) *Config {
	if dir == "" {
		return c
	}
	c.WorkDir = dir
	return c
}

func (c *Config) WithNetwork(mode string) *Config {
	if mode == "" {
		return c
	}
	c.Network = mode
	return c
}

func (c *Config) WithContainerDiskSpec(spec *ContainerDiskSpec) *Config {
	if spec == nil {
		return c
	}
	if spec.Path == "" {
		return c
	}
	specCopy := *spec
	c.ContainerDisk = &specCopy
	return c
}
func (c *Config) WithPodmanProxyAPIFile(path string) *Config {
	if path == "" {
		return c
	}
	c.PodmanProxyAPIFile = path
	return c
}
func (c *Config) WithManageAPIFile(path string) *Config {
	if path == "" {
		return c
	}
	c.ManageAPIFile = path
	return c
}

func (c *Config) WithExportSSHKeyPrivateFile(path string) *Config {
	if path == "" {
		return c
	}

	c.SSHKeyFileSymbolPath = path
	return c
}

func (c *Config) WithEventReporter(reportURL string) *Config {
	if reportURL == "" {
		return c
	}
	c.ReportURL = reportURL
	return c
}

func (c *Config) WithPortForwards(forwards ...define.PortForward) *Config {
	if len(forwards) == 0 {
		return c
	}
	c.PortForwards = append(c.PortForwards, forwards...)
	return c
}

func (c *Config) WithPortUnforwards(forwards ...define.PortForward) *Config {
	if len(forwards) == 0 {
		return c
	}
	c.PortUnforwards = append(c.PortUnforwards, forwards...)
	return c
}

func (c *Config) WithProxy(enable bool) *Config {
	logrus.Infof("get proxy setting from system: %v", enable)
	c.Proxy = enable
	return c
}

func (c *Config) WithCommand(bin string, args ...string) *Config {
	if bin == "" {
		return c
	}
	return c.WithCommandLine(append([]string{bin}, args...)...)
}

func (c *Config) WithCommandLine(cmdline ...string) *Config {
	if len(cmdline) == 0 || cmdline[0] == "" {
		return c
	}
	c.Command = append([]string(nil), cmdline...)
	return c
}

func (c *Config) WithPTY(enable bool) *Config {
	c.PTY = enable
	return c
}

func (c *Config) WithEnv(kvs ...string) *Config {
	if len(kvs) == 0 {
		return c
	}
	for _, kv := range kvs {
		if kv == "" {
			continue
		}
		c.Env = append(c.Env, kv)
	}
	return c
}

func (c *Config) WithMount(specs ...string) *Config {
	if len(specs) == 0 {
		return c
	}
	for _, spec := range specs {
		if spec == "" {
			continue
		}
		c.Mounts = append(c.Mounts, spec)
	}
	return c
}

func (c *Config) WithRawDiskSpecs(specs ...RawDiskSpec) *Config {
	if len(specs) == 0 {
		return c
	}
	for _, spec := range specs {
		if spec.Path == "" {
			continue
		}
		c.Disks = append(c.Disks, spec)
	}
	return c
}

// --- Loading ---------------------------------------------------------------

// WriteCfg marshals cfg as JSON and writes it to path.
func (c *Config) WriteCfg(path string) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
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
	if len(cfg.PortForwards) == 0 && len(cfg.PortUnforwards) == 0 {
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

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func RandomString() string {
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
