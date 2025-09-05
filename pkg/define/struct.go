package define

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

type Mount struct {
	ReadOnly bool   `json:"readOnly"`
	Source   string `json:"source"`
	Tag      string `json:"tag"`
	Target   string `json:"target"`
	Type     string `json:"type"`
}

type SSHInfo struct {
	User      string `json:"user"`
	HostPort  uint64 `json:"hostPort"`
	HostAddr  string `json:"hostAddr"`
	GuestAddr string `json:"guestAddr"`
	GuestPort uint64 `json:"guestPort"`

	// HostSSHKeyPairFile is the path to the host ssh keypair file
	HostSSHKeyPairFile string `json:"hostSSHKeyFile,omitempty"`

	HostSSHPublicKey  string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey string `json:"sshPrivateKey,omitempty"`
}

type VMConfig struct {
	MemoryInMB int32  `json:"memoryInMB,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX
	DataDisk []*DataDisk `json:"dataDisk,omitempty"`
	// GVproxy control endpoint
	GVproxyEndpoint string `json:"GVproxyEndpoint,omitempty"`
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string     `json:"networkStackBackend,omitempty"`
	LogLevel            string     `json:"logLevel,omitempty"`
	Mounts              []Mount    `json:"mounts,omitempty"`
	SSHInfo             SSHInfo    `json:"sshInfo,omitempty"`
	Cmdline             Cmdline    `json:"cmdline,omitempty"`
	PodmanInfo          PodmanInfo `json:"podmanInfo,omitempty"`

	// whatever the guest network is ready
	NetworkReadyChan chan bool `json:"-"`
}

type DataDisk struct {
	UUID           string `json:"uuid"`
	FileSystemType string `json:"filesystemType"`
	Path           string `json:"path"`
}

type PodmanInfo struct {
	UnixSocksAddr string `json:"unixSocksAddr,omitempty"`
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	// Bootstrap is a process that runs under PID 1. As a secondary init, Bootstrap incubates all user child processes.
	Bootstrap     string   `json:"bootstrap,omitempty"`
	BootstrapArgs []string `json:"bootstrapArgs,omitempty"`
	// Support two modes: define.RunUserCommandLineMode and define.RunDockerEngineMode
	Mode      string `json:"mode,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	// TargetBin is the binary to run by bootstrap.
	TargetBin string `json:"targetBin,omitempty"`
	// TargetBinArgs is the arguments to pass to the target binary.
	TargetBinArgs []string `json:"targetBinArgs,omitempty"`
	// Env is the environment variables to set for the bootstrap process and target binary, in the form of KEY=VALUE.
	Env []string `json:"env,omitempty"`
}

func LoadVMCFromFile(file string) (*VMConfig, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	vmc := &VMConfig{}

	if err = json.NewDecoder(f).Decode(vmc); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %w", file, err)
	}
	return vmc, nil
}
