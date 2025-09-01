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
	User                 string `json:"user"`
	HostPort             uint64 `json:"hostPort"`
	HostAddr             string `json:"hostAddr"`
	GuestAddr            string `json:"guestAddr"`
	GuestPort            uint64 `json:"guestPort"`
	AuthorizationKeyFile string `json:"authorizationKey"`
}

type VMConfig struct {
	MemoryInMB int32  `json:"memoryInMB,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX
	DataDisk []string `json:"dataDisk,omitempty"`
	// GVproxy control endpoint
	GVproxyEndpoint string `json:"GVproxyEndpoint,omitempty"`
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string  `json:"networkStackBackend,omitempty"`
	LogLevel            string  `json:"logLevel,omitempty"`
	Mounts              []Mount `json:"mounts,omitempty"`
	SSHInfo             SSHInfo
	HostSSHKeyFile      string `json:"hostSSHKeyFile,omitempty"`
	HostSSHPublicKey    string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey   string `json:"sshPrivateKey,omitempty"`
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	Workspace     string   `json:"workspace,omitempty"`
	TargetBin     string   `json:"targetBin,omitempty"`
	TargetBinArgs []string `json:"targetBinArgs,omitempty"`
	Env           []string `json:"env,omitempty"`
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
