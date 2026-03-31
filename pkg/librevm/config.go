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
	ModeCfgGen    RunMode = "cfggen"
)

func (m RunMode) IsValid() bool {
	switch m {
	case ModeRootfs, ModeContainer:
		return true
	default:
		return false
	}
}

type Config struct {
	RunMode   RunMode `toml:"runMode,omitempty" json:"runMode,omitempty"`
	SessionID string  `toml:"sessionID,omitempty" json:"sessionID,omitempty"` // session name
	CPUs      int     `toml:"cpus,omitempty"      json:"cpus,omitempty"`      // 0 → host CPU count
	MemoryMB  uint64  `toml:"memory_mb,omitempty" json:"memoryMB,omitempty"`  // 0 → host total RAM
	Rootfs    string  `toml:"rootfs,omitempty"    json:"rootfs,omitempty"`    // empty → built-in Alpine

	// Command specifies the program to run inside the VM (rootfs mode only).
	Command []string `toml:"command,omitempty"  json:"command,omitempty"`
	WorkDir string   `toml:"workdir,omitempty"  json:"workdir,omitempty"`
	Env     []string `toml:"env,omitempty"      json:"env,omitempty"`

	Network                 string             `toml:"network,omitempty"         json:"network,omitempty"` // "gvisor" | "tsi"
	Mounts                  []string           `toml:"mounts,omitempty"          json:"mounts,omitempty"`  // "/host:/guest[,ro]"
	Disks                   []RawDiskSpec      `toml:"disks,omitempty"           json:"disks,omitempty"`
	ContainerDisk           *ContainerDiskSpec `toml:"container_disk,omitempty" json:"containerDisk,omitempty"`
	PodmanProxyAPIFile      string             `toml:"podman_proxy_api_file,omitempty"   json:"podmanProxyAPIFile,omitempty"`
	ManageAPIFile           string             `toml:"manage_api_file,omitempty"         json:"manageAPIFile,omitempty"`
	SSHKeyDir               string             `toml:"ssh_key_dir,omitempty"                json:"sshKeyDir,omitempty"`
	ExportSSHKeyPrivateFile string             `toml:"export_ssh_key_private_file,omitempty" json:"exportSSHKeyPrivateFile,omitempty"`
	ExportSSHKeyPublicFile  string             `toml:"export_ssh_key_public_file,omitempty"  json:"exportSSHKeyPublicFile,omitempty"`
	Proxy                   bool               `toml:"proxy,omitempty"           json:"proxy,omitempty"`
	LogLevel                string             `toml:"log_level,omitempty"       json:"logLevel,omitempty"` // default "info"
	LogTo                   string             `toml:"log_to,omitempty"          json:"logTo,omitempty"`
	Reporters               []EventReporter    `toml:"-" json:"-"`
}

// DefaultConfig returns a Config with sensible defaults pre-filled.
// Zero-value resource fields (CPUs, MemoryMB) are resolved at VM creation time.
func DefaultConfig(sessionID string) *Config {
	if sessionID == "" {
		sessionID = RandomString()
		logrus.Warnf("DefaultConfig: session name is empty, autogenerate a random one: %s", sessionID)
	}

	return &Config{
		Network:   "gvisor",
		LogLevel:  "info",
		WorkDir:   "/",
		SessionID: sessionID,
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

func (c *Config) WithSSHKeyDir(dir string) *Config {
	if dir == "" {
		return c
	}
	c.SSHKeyDir = dir
	return c
}
func (c *Config) WithExportSSHKeyPrivateFile(path string) *Config {
	if path == "" {
		return c
	}
	c.ExportSSHKeyPrivateFile = path
	return c
}
func (c *Config) WithExportSSHKeyPublicFile(path string) *Config {
	if path == "" {
		return c
	}
	c.ExportSSHKeyPublicFile = path
	return c
}
func (c *Config) WithEventReporter(reporters ...EventReporter) *Config {
	if len(reporters) == 0 {
		return c
	}
	for _, r := range reporters {
		if r == nil {
			continue
		}
		c.Reporters = append(c.Reporters, r)
	}
	return c
}

func (c *Config) WithProxy(enable bool) *Config {
	logrus.Infof("get proxy setting from system: %v", enable)
	c.Proxy = enable
	return c
}

const maxLogFileSize = 10 * 1024 * 1024

func (c *Config) WithLogSetup(level string, logFilePath string) *Config {
	if level == "" {
		level = logrus.InfoLevel.String()
		logrus.Infof("default log level: %q", level)
	}

	// setup logrus
	l, err := logrus.ParseLevel(level)
	if err != nil {
		l = logrus.InfoLevel
		logrus.Warnf("failed to parse log level: %v, using default log level %s", err, l.String())
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})

	c.LogLevel = level

	if logFilePath == "" {
		logFilePath = filepath.Join(getSessionDir(c.SessionID), "logs", "revm.log")
		logrus.Infof("default log file path: %q", logFilePath)
	} else {
		logrus.Infof("custom log file path: %q", logFilePath)
	}

	if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
		logrus.Warnf("Config.WithLogSetup failed to create log directory: %v", err)
		return c
	}

	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogFileSize {
		_ = os.Truncate(logFilePath, 0)
	}

	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logrus.Warnf("Config.WithLogSetup failed to open log file: %v", err)
		return c
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))

	c.LogTo = logFilePath

	logrus.Infof("start virtualMachine, full cmdline: %q", os.Args)
	return c
}

func (c *Config) WithCommand(bin string, args ...string) *Config {
	if bin == "" {
		return c
	}
	c.Command = append([]string{bin}, args...)
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
