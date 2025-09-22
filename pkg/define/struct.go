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
	Opts     string `json:"opts"`
	UUID     string `json:"uuid"`
}

type SSHInfo struct {
	// HostSSHKeyPairFile is the path to the host ssh keypair file
	HostSSHKeyPairFile string `json:"hostSSHKeyFile,omitempty"`

	HostSSHPublicKey  string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey string `json:"sshPrivateKey,omitempty"`
}

type VMConfig struct {
	MemoryInMB    uint64   `json:"memoryInMB,omitempty"`
	Cpus          int8     `json:"cpus,omitempty"`
	RootFS        string   `json:"rootFS,omitempty"`
	Kernel        string   `json:"kernel,omitempty"`
	Initrd        string   `json:"initrd,omitempty"`
	KernelCmdline []string `json:"kernelArgs,omitempty"`

	// data disk will map into /dev/vdX and automount by bootstrap process
	DataDisk []DataDisk `json:"dataDisk,omitempty"`
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
	RestAPIAddress      string     `json:"restAPIAddress,omitempty"`
	// RunMode is the mode of the VM, which can be define.RunUserRootfsMode, define.RunDirectBootKernelMode or define.RunDockerEngineMode
	RunMode            string `json:"runMode,omitempty"`
	IgnProvisionerAddr string `json:"ignProvisionerAddr,omitempty"`

	Stage Stage `json:"-"`
}

type Stage struct {
	// when gvproxy is running, this channel will be closed
	GVProxyChan chan struct{}
	// when ignition server is running, this channel will be closed
	IgnServerChan   chan struct{}
	PodmanReadyChan chan struct{}
	SSHDReadyChan   chan struct{}
}

// DataDisk represents the configuration of a data disk, including its file system type, path, and mount point.
type DataDisk struct {
	IsContainerStorage bool   `json:"isContainerStorage,omitempty"`
	FsType             string `json:"fsType,omitempty"`
	UUID               string `json:"UUID,omitempty"`
	Path               string `json:"path,omitempty"`
	MountTo            string `json:"mountTo,omitempty"`
	SizeInGB           uint64 `json:"sizeInGB,omitempty"`
}

type PodmanInfo struct {
	UnixSocksAddr string `json:"unixSocksAddr,omitempty"`
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	// Bootstrap is a process that runs under PID 1. As a secondary init, Bootstrap incubates all user child processes.
	Bootstrap     string   `json:"bootstrap,omitempty"`
	BootstrapArgs []string `json:"bootstrapArgs,omitempty"`
	Workspace     string   `json:"workspace,omitempty"`
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

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}
