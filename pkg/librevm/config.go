//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Mode selects the VM run mode.
type Mode string

const (
	// ModeRootfs boots the VM with a rootfs and executes a command.
	ModeRootfs Mode = "rootfs"
	// ModeContainer boots the VM with the built-in container runtime (Podman).
	ModeContainer Mode = "container"
)

// Config declares the complete VM specification. Zero-value fields use
// sensible defaults (filled in during [New]).
// Config is serializable as TOML and JSON, and can also be built using the
// fluent With* chain methods.
type Config struct {
	Mode     Mode   `toml:"mode"               json:"mode"` // required: "rootfs" | "container"
	Name     string `toml:"name,omitempty"      json:"name,omitempty"`
	CPUs     int    `toml:"cpus,omitempty"      json:"cpus,omitempty"`     // 0 → host CPU count
	MemoryMB uint64 `toml:"memory_mb,omitempty" json:"memoryMB,omitempty"` // 0 → host total RAM
	Rootfs   string `toml:"rootfs,omitempty"    json:"rootfs,omitempty"`   // empty → built-in Alpine

	// Command specifies the program to run inside the VM (rootfs mode only).
	Command []string `toml:"command,omitempty"  json:"command,omitempty"`
	WorkDir string   `toml:"workdir,omitempty"  json:"workdir,omitempty"`
	Env     []string `toml:"env,omitempty"      json:"env,omitempty"`

	Network       string   `toml:"network,omitempty"         json:"network,omitempty"` // "gvisor" | "tsi"
	Mounts        []string `toml:"mounts,omitempty"          json:"mounts,omitempty"`  // "/host:/guest[,ro]"
	Disks         []string `toml:"disks,omitempty"           json:"disks,omitempty"`   // ext4 paths
	ContainerDisk string   `toml:"container_disk,omitempty"  json:"containerDisk,omitempty"`
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

func (c *Config) WithMode(m Mode) *Config               { c.Mode = m; return c }
func (c *Config) WithName(name string) *Config          { c.Name = name; return c }
func (c *Config) WithCPUs(n int) *Config                { c.CPUs = n; return c }
func (c *Config) WithMemory(mb uint64) *Config          { c.MemoryMB = mb; return c }
func (c *Config) WithRootfs(path string) *Config        { c.Rootfs = path; return c }
func (c *Config) WithWorkDir(dir string) *Config        { c.WorkDir = dir; return c }
func (c *Config) WithNetwork(mode string) *Config       { c.Network = mode; return c }
func (c *Config) WithContainerDisk(path string) *Config { c.ContainerDisk = path; return c }
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
