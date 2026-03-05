//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/shirou/gopsutil/v4/mem"
)

// RunMode selects the VM run mode.
type RunMode string

const (
	// ModeRootfs boots the VM with a rootfs and executes a command.
	ModeRootfs RunMode = "rootfs"
	// ModeContainer boots the VM with the built-in container runtime (Podman).
	ModeContainer RunMode = "docker"

	// ovm 模式特有的 RunMode
	ModeOVMRun  RunMode = "run"
	ModeOVMInit RunMode = "init"
)

func (m RunMode) IsOVMRun() bool { return m == ModeOVMRun }

func (m RunMode) IsOVMInit() bool { return m == ModeOVMInit }

func (m RunMode) IsOVM() bool { return m.IsOVMRun() || m.IsOVMInit() }

func (m RunMode) IsContainerLike() bool { return m == ModeContainer || m.IsOVM() }

func (m RunMode) IsValid() bool {
	switch m {
	case ModeRootfs, ModeContainer, ModeOVMRun, ModeOVMInit:
		return true
	default:
		return false
	}
}

// Config declares the complete VM specification. Zero-value fields use
// sensible defaults (filled in during [New]).
// Config is serializable as TOML and JSON, and can also be built using the
// fluent With* chain methods.
type Config struct {
	RunMode   RunMode `toml:"runMode" json:"runMode"`                         // required: "rootfs" | "docker" | "run" | "init"
	SessionID string  `toml:"sessionID,omitempty" json:"sessionID,omitempty"` // session name
	CPUs      int     `toml:"cpus,omitempty"      json:"cpus,omitempty"`      // 0 → host CPU count
	MemoryMB  uint64  `toml:"memory_mb,omitempty" json:"memoryMB,omitempty"`  // 0 → host total RAM
	Rootfs    string  `toml:"rootfs,omitempty"    json:"rootfs,omitempty"`    // empty → built-in Alpine

	// Command specifies the program to run inside the VM (rootfs mode only).
	Command []string `toml:"command,omitempty"  json:"command,omitempty"`
	WorkDir string   `toml:"workdir,omitempty"  json:"workdir,omitempty"`
	Env     []string `toml:"env,omitempty"      json:"env,omitempty"`

	Network       string   `toml:"network,omitempty"         json:"network,omitempty"` // "gvisor" | "tsi"
	Mounts        []string `toml:"mounts,omitempty"          json:"mounts,omitempty"`  // "/host:/guest[,ro]"
	Disks         []string `toml:"disks,omitempty"           json:"disks,omitempty"`   // ext4 paths
	ContainerDisk        string `toml:"container_disk,omitempty"         json:"containerDisk,omitempty"`
	ContainerDiskVersion string `toml:"container_disk_version,omitempty" json:"containerDiskVersion,omitempty"`
	PodmanProxyAPI       string `toml:"podman_proxy_api,omitempty"       json:"podmanProxyAPI,omitempty"`
	Proxy         bool     `toml:"proxy,omitempty"           json:"proxy,omitempty"`
	LogLevel      string   `toml:"log_level,omitempty"       json:"logLevel,omitempty"` // default "info"
	ReportURL     string   `toml:"report_url,omitempty"      json:"reportURL,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults pre-filled.
// Zero-value resource fields (CPUs, MemoryMB) are resolved at VM creation time.
func DefaultConfig() *Config {
	return &Config{
		Network:  "gvisor",
		LogLevel: "info",
		WorkDir:  "/",
	}
}

// --- Chain (fluent) methods ------------------------------------------------

func (c *Config) WithMode(m RunMode) *Config            { c.RunMode = m; return c }
func (c *Config) WithName(name string) *Config          { c.SessionID = name; return c }
func (c *Config) WithCPUs(n int) *Config                { c.CPUs = n; return c }
func (c *Config) WithMemory(mb uint64) *Config          { c.MemoryMB = mb; return c }
func (c *Config) WithRootfs(path string) *Config        { c.Rootfs = path; return c }
func (c *Config) WithWorkDir(dir string) *Config        { c.WorkDir = dir; return c }
func (c *Config) WithNetwork(mode string) *Config       { c.Network = mode; return c }
func (c *Config) WithContainerDisk(path string) *Config { c.ContainerDisk = path; return c }
func (c *Config) WithContainerDiskVersion(v string) *Config {
	c.ContainerDiskVersion = v
	return c
}
func (c *Config) WithPodmanProxyAPI(path string) *Config { c.PodmanProxyAPI = path; return c }
func (c *Config) WithProxy(enable bool) *Config         { c.Proxy = enable; return c }
func (c *Config) WithLogLevel(level string) *Config     { c.LogLevel = level; return c }

func (c *Config) WithCommand(bin string, args ...string) *Config {
	c.Command = append([]string{bin}, args...)
	return c
}

func (c *Config) WithEnv(kvs ...string) *Config {
	c.Env = append(c.Env, kvs...)
	return c
}

func (c *Config) WithMount(specs ...string) *Config {
	c.Mounts = append(c.Mounts, specs...)
	return c
}

func (c *Config) WithDisk(paths ...string) *Config {
	c.Disks = append(c.Disks, paths...)
	return c
}

// --- Loading ---------------------------------------------------------------

// LoadFile reads a Config from path. The format is detected by extension:
// .toml for TOML, .json for JSON. Any other extension returns an error.
func LoadFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return Load(f)
	case ".json":
		return loadJSON(f)
	default:
		return nil, fmt.Errorf("unsupported config format %q (use .toml or .json)", ext)
	}
}

// Load reads a TOML-encoded Config from r.
func Load(r io.Reader) (*Config, error) {
	var cfg Config
	if _, err := toml.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode toml config: %w", err)
	}
	return &cfg, nil
}

func loadJSON(r io.Reader) (*Config, error) {
	var cfg Config
	if err := json.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode json config: %w", err)
	}
	return &cfg, nil
}

// --- Normalization & Validation --------------------------------------------

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
