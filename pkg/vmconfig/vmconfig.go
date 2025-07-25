package vmconfig

import (
	"encoding/json"
	"fmt"
	"linuxvm/pkg/filesystem"
	"os"
)

// VMConfig Static virtual machine configuration.
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
	NetworkStackBackend string             `json:"networkStackBackend,omitempty"`
	LogLevel            string             `json:"logLevel,omitempty"`
	Mounts              []filesystem.Mount `json:"mounts,omitempty"`
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	Workspace     string   `json:"workspace,omitempty"`
	TargetBin     string   `json:"targetBin,omitempty"`
	TargetBinArgs []string `json:"targetBinArgs,omitempty"`
	Env           []string `json:"env,omitempty"`
}

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}
