package define

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

type VMConfig struct {
	WorkspacePath string `json:"workspacePath,omitempty"`

	MemoryInMB uint64 `json:"memoryInMB,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX and automount by guest-agent process
	BlkDevs []BlkDev `json:"blkDevs,omitempty"`
	// GVproxy control endpoint
	GvisorTapVsockEndpoint string `json:"GvisorTapVsockEndpoint,omitempty"`
	// GvisorTapVsockNetwork is the network stack backend to use. which provided
	// by gvproxy
	GvisorTapVsockNetwork string            `json:"gvisorTapVsockNetwork,omitempty"`
	LogFilePath           string            `json:"logFilePath,omitempty"`
	Mounts                []Mount           `json:"mounts,omitempty"`
	SSHInfo               SSHInfo           `json:"sshInfo,omitempty"`
	PodmanInfo            PodmanInfo        `json:"podmanInfo,omitempty"`
	VMCtlAddress          string            `json:"vmCTLAddress,omitempty"`
	RunMode               string            `json:"runMode,omitempty"`
	IgnitionServerCfg     IgnitionServerCfg `json:"ignitionServerCfg,omitempty"`
	GuestAgentCfg         GuestAgentCfg     `json:"guestAgentCfg,omitempty"`
	Cmdline               Cmdline           `json:"cmdline,omitempty"`
}

type Cmdline struct {
	Envs []string `json:"envs,omitempty"`
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
}

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
	// HostSSHPrivateKeyFile is the path to the host ssh private key
	HostSSHPrivateKeyFile string `json:"hostSSHKeyFile,omitempty"`

	HostSSHPublicKey  string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey string `json:"sshPrivateKey,omitempty"`

	SSHLocalForwardAddr string `json:"sshLocalForwardAddr,omitempty"`
}

type ProxySetting struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	Use        bool   `json:"use,omitempty"`
}

type IgnitionServerCfg struct {
	ListenUnixSockAddr string `json:"ListenUnixSockAddr,omitempty"`
	ListenTcpAddr      string `json:"ListenTcpAddr,omitempty"` // not implemented yet
}

type LinuxTools struct {
	Busybox     string `json:"busybox,omitempty"`
	DropBear    string `json:"dropbear,omitempty"`
	DropBearKey string `json:"dropbearkey,omitempty"`
	GuestAgent  string `json:"guestAgent,omitempty"`
}

type DarwinTools struct {
	E2fsck  string `json:"e2fsck,omitempty"`
	Blkid   string `json:"blkid,omitempty"`
	Tune2fs string `json:"tune2fs,omitempty"`
}

type ExternalTools struct {
	LinuxTools  LinuxTools  `json:"linuxTools,omitempty"`
	DarwinTools DarwinTools `json:"darwinTools,omitempty"`
}

// BlkDev represents the configuration of a data disk, including its file system type, path, and mount point.
type BlkDev struct {
	FsType    string `json:"fsType,omitempty"`
	UUID      string `json:"UUID,omitempty"`
	Path      string `json:"path,omitempty"`
	MountTo   string `json:"mountTo,omitempty"`
	SizeInMib uint64 `json:"sizeInMIB,omitempty"`
}

type PodmanInfo struct {
	LocalPodmanProxyAddr string `json:"localPodmanProxyAddr,omitempty"`
	GuestPodmanAPIIP     string `json:"GuestPodmanAPIIP,omitempty"`
	GuestPodmanAPIPort   uint16 `json:"GuestPodmanAPIPort,omitempty"`
}

type GuestAgentCfg struct {
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
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
