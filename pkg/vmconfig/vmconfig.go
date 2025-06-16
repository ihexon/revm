package vmconfig

import (
	"encoding/json"
	"fmt"
	"linuxvm/pkg/filesystem"
	"os"
)

// VMConfig Static virtual machine configuration.
type VMConfig struct {
	CtxID      uint32
	MemoryInMB int32
	Cpus       int8
	RootFS     string

	// data disk will map into /dev/vdX
	DataDisk []string
	// GVproxy control endpoint
	GVproxyEndpoint string
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string
	LogLevel            string
	Mounts              []filesystem.Mount
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	Workspace     string
	TargetBin     string
	TargetBinArgs []string
	Env           []string
}

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)

}
