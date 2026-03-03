//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Network != "gvisor" {
		t.Errorf("Network = %q, want %q", cfg.Network, "gvisor")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.WorkDir != "/" {
		t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, "/")
	}
}

func TestConfigChain(t *testing.T) {
	cfg := DefaultConfig().
		WithMode(ModeRootfs).
		WithName("test-vm").
		WithCPUs(4).
		WithMemory(2048).
		WithRootfs("/my/rootfs").
		WithCommand("/bin/sh", "-c", "echo hello").
		WithWorkDir("/workspace").
		WithEnv("FOO=bar", "BAZ=qux").
		WithMount("/src:/src", "/data:/data,ro").
		WithDisk("/var/data/disk.ext4").
		WithNetwork("tsi").
		WithContainerDisk("/var/lib/storage.ext4").
		WithProxy(true).
		WithLogLevel("debug")

	if cfg.Mode != ModeRootfs {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeRootfs)
	}
	if cfg.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-vm")
	}
	if cfg.CPUs != 4 {
		t.Errorf("CPUs = %d, want %d", cfg.CPUs, 4)
	}
	if cfg.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, 2048)
	}
	if cfg.Rootfs != "/my/rootfs" {
		t.Errorf("Rootfs = %q, want %q", cfg.Rootfs, "/my/rootfs")
	}
	if len(cfg.Command) != 3 || cfg.Command[0] != "/bin/sh" {
		t.Errorf("Command = %v, want [/bin/sh -c echo hello]", cfg.Command)
	}
	if cfg.WorkDir != "/workspace" {
		t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, "/workspace")
	}
	if len(cfg.Env) != 2 || cfg.Env[0] != "FOO=bar" {
		t.Errorf("Env = %v, want [FOO=bar BAZ=qux]", cfg.Env)
	}
	if len(cfg.Mounts) != 2 {
		t.Errorf("Mounts = %v, want 2 items", cfg.Mounts)
	}
	if len(cfg.Disks) != 1 || cfg.Disks[0] != "/var/data/disk.ext4" {
		t.Errorf("Disks = %v, want [/var/data/disk.ext4]", cfg.Disks)
	}
	if cfg.Network != "tsi" {
		t.Errorf("Network = %q, want %q", cfg.Network, "tsi")
	}
	if cfg.ContainerDisk != "/var/lib/storage.ext4" {
		t.Errorf("ContainerDisk = %q, want %q", cfg.ContainerDisk, "/var/lib/storage.ext4")
	}
	if !cfg.Proxy {
		t.Error("Proxy = false, want true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestConfigMode(t *testing.T) {
	cfg := DefaultConfig().WithMode(ModeRootfs)
	if cfg.Mode != ModeRootfs {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeRootfs)
	}

	cfg.WithMode(ModeContainer)
	if cfg.Mode != ModeContainer {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeContainer)
	}
}

func TestLoadTOML(t *testing.T) {
	input := `
mode      = "rootfs"
name      = "build-job"
cpus      = 4
memory_mb = 4096
workdir   = "/workspace"
command   = ["make", "-j4"]
env       = ["CC=gcc", "CFLAGS=-O2"]
network   = "gvisor"
proxy     = true
log_level = "debug"

mounts = [
    "/home/user/src:/workspace",
    "/home/user/.cache:/root/.cache,ro",
]

disks = ["/var/data/build-cache.ext4"]
`
	cfg, err := Load(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Mode != ModeRootfs {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeRootfs)
	}
	if cfg.Name != "build-job" {
		t.Errorf("Name = %q, want %q", cfg.Name, "build-job")
	}
	if cfg.CPUs != 4 {
		t.Errorf("CPUs = %d, want %d", cfg.CPUs, 4)
	}
	if cfg.MemoryMB != 4096 {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, 4096)
	}
	if cfg.WorkDir != "/workspace" {
		t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, "/workspace")
	}
	if len(cfg.Command) != 2 || cfg.Command[0] != "make" {
		t.Errorf("Command = %v, want [make -j4]", cfg.Command)
	}
	if len(cfg.Env) != 2 {
		t.Errorf("Env = %v, want 2 items", cfg.Env)
	}
	if cfg.Network != "gvisor" {
		t.Errorf("Network = %q, want %q", cfg.Network, "gvisor")
	}
	if !cfg.Proxy {
		t.Error("Proxy = false, want true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if len(cfg.Mounts) != 2 {
		t.Errorf("Mounts = %v, want 2 items", cfg.Mounts)
	}
	if len(cfg.Disks) != 1 {
		t.Errorf("Disks = %v, want 1 item", cfg.Disks)
	}
}

func TestLoadTOMLContainerMode(t *testing.T) {
	input := `
mode           = "container"
name           = "dev-engine"
cpus           = 8
memory_mb      = 8192
network        = "tsi"
container_disk = "/var/lib/revm/storage.ext4"
mounts         = ["/home/user:/home/user"]
`
	cfg, err := Load(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Name != "dev-engine" {
		t.Errorf("Name = %q, want %q", cfg.Name, "dev-engine")
	}
	if cfg.Mode != ModeContainer {
		t.Errorf("Mode = %q, want %q", cfg.Mode, ModeContainer)
	}
	if cfg.ContainerDisk != "/var/lib/revm/storage.ext4" {
		t.Errorf("ContainerDisk = %q, want %q", cfg.ContainerDisk, "/var/lib/revm/storage.ext4")
	}
}

func TestLoadJSON(t *testing.T) {
	input := `{
		"mode": "rootfs",
		"name": "json-vm",
		"cpus": 2,
		"memoryMB": 1024,
		"network": "gvisor",
		"command": ["/bin/echo", "hello"]
	}`
	cfg, err := loadJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("loadJSON: %v", err)
	}
	if cfg.Name != "json-vm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "json-vm")
	}
	if cfg.CPUs != 2 {
		t.Errorf("CPUs = %d, want %d", cfg.CPUs, 2)
	}
	if cfg.MemoryMB != 1024 {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, 1024)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing mode",
			cfg:     Config{CPUs: 1, MemoryMB: 1024, Network: "gvisor"},
			wantErr: "mode must be",
		},
		{
			name:    "invalid mode",
			cfg:     Config{Mode: "bogus", CPUs: 1, MemoryMB: 1024, Network: "gvisor"},
			wantErr: "mode must be",
		},
		{
			name:    "rootfs without command",
			cfg:     Config{Mode: ModeRootfs, CPUs: 1, MemoryMB: 1024, Network: "gvisor"},
			wantErr: "rootfs mode requires a non-empty command",
		},
		{
			name:    "rootfs with empty binary",
			cfg:     Config{Mode: ModeRootfs, CPUs: 1, MemoryMB: 1024, Network: "gvisor", Command: []string{""}},
			wantErr: "rootfs mode requires a non-empty command",
		},
		{
			name:    "memory too low",
			cfg:     Config{Mode: ModeContainer, CPUs: 1, MemoryMB: 256, Network: "gvisor"},
			wantErr: "memory must be at least 512 MB",
		},
		{
			name:    "cpus too low",
			cfg:     Config{Mode: ModeContainer, CPUs: 0, MemoryMB: 1024, Network: "gvisor"},
			wantErr: "cpus must be at least 1",
		},
		{
			name:    "invalid network",
			cfg:     Config{Mode: ModeContainer, CPUs: 1, MemoryMB: 1024, Network: "invalid"},
			wantErr: "network must be",
		},
		{
			name: "valid rootfs config",
			cfg:  Config{Mode: ModeRootfs, CPUs: 4, MemoryMB: 2048, Network: "gvisor", Command: []string{"/bin/sh"}},
		},
		{
			name: "valid container config",
			cfg:  Config{Mode: ModeContainer, CPUs: 4, MemoryMB: 2048, Network: "tsi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("validate() = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("validate() = %q, want error containing %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestResolveDefaults(t *testing.T) {
	cfg, err := NormalizeConfig(Config{})
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}

	if cfg.Name == "" {
		t.Error("Name should be filled with random string")
	}
	if cfg.CPUs < 1 {
		t.Errorf("CPUs = %d, want >= 1", cfg.CPUs)
	}
	if cfg.MemoryMB < 512 {
		t.Errorf("MemoryMB = %d, want >= 512", cfg.MemoryMB)
	}
	if cfg.Network != "gvisor" {
		t.Errorf("Network = %q, want %q", cfg.Network, "gvisor")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.WorkDir != "/" {
		t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, "/")
	}
}

func TestResolveDefaultsPreservesExplicitValues(t *testing.T) {
	input := Config{
		Name:     "my-vm",
		CPUs:     8,
		MemoryMB: 4096,
		Network:  "tsi",
		LogLevel: "debug",
		WorkDir:  "/workspace",
	}
	cfg, err := NormalizeConfig(input)
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}

	if cfg.Name != "my-vm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "my-vm")
	}
	if cfg.CPUs != 8 {
		t.Errorf("CPUs = %d, want %d", cfg.CPUs, 8)
	}
	if cfg.MemoryMB != 4096 {
		t.Errorf("MemoryMB = %d, want %d", cfg.MemoryMB, 4096)
	}
	if cfg.Network != "tsi" {
		t.Errorf("Network = %q, want %q", cfg.Network, "tsi")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.WorkDir != "/workspace" {
		t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, "/workspace")
	}
}

func TestNormalizeConfigDoesNotMutateInput(t *testing.T) {
	in := Config{}
	_, err := NormalizeConfig(in)
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	if in.Name != "" || in.CPUs != 0 || in.MemoryMB != 0 {
		t.Fatalf("input config mutated: %+v", in)
	}
}

func TestRandomName(t *testing.T) {
	name := randomName()
	if len(name) != 8 {
		t.Errorf("randomName() length = %d, want 8", len(name))
	}

	// Ensure two calls produce different names (probabilistic but virtually certain).
	name2 := randomName()
	if name == name2 {
		t.Errorf("randomName() returned same value twice: %q", name)
	}
}

func TestWithMountAppends(t *testing.T) {
	cfg := DefaultConfig().
		WithMount("/a:/a").
		WithMount("/b:/b", "/c:/c")
	if len(cfg.Mounts) != 3 {
		t.Errorf("Mounts = %v, want 3 items", cfg.Mounts)
	}
}

func TestWithEnvAppends(t *testing.T) {
	cfg := DefaultConfig().
		WithEnv("A=1").
		WithEnv("B=2", "C=3")
	if len(cfg.Env) != 3 {
		t.Errorf("Env = %v, want 3 items", cfg.Env)
	}
}

func TestWithDiskAppends(t *testing.T) {
	cfg := DefaultConfig().
		WithDisk("/d1.ext4").
		WithDisk("/d2.ext4", "/d3.ext4")
	if len(cfg.Disks) != 3 {
		t.Errorf("Disks = %v, want 3 items", cfg.Disks)
	}
}
