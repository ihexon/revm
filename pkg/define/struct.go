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
	// HostSSHPrivateKeyFile is the path to the host ssh private key
	HostSSHPrivateKeyFile string `json:"hostSSHKeyFile,omitempty"`

	HostSSHPublicKey  string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey string `json:"sshPrivateKey,omitempty"`

	SSHLocalForwardAddr string `json:"sshLocalForwardAddr,omitempty"`
}

type LogKeyType string

const (
	GlobalLogKey LogKeyType = "global"
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
	GvisorTapVsockNetwork string        `json:"gvisorTapVsockNetwork,omitempty"`
	LogFilePath           string        `json:"logFilePath,omitempty"`
	Mounts                []Mount       `json:"mounts,omitempty"`
	SSHInfo               SSHInfo       `json:"sshInfo,omitempty"`
	PodmanInfo            PodmanInfo    `json:"podmanInfo,omitempty"`
	VMCtlAddress          string        `json:"vmCTLAddress,omitempty"`
	RunMode               string        `json:"runMode,omitempty"`
	Ignition              Ignition      `json:"ignition,omitempty"`
	GuestAgentCfg         GuestAgentCfg `json:"guestAgentCfg,omitempty"`
}

type Ignition struct {
	HostListenAddr string `json:"HostListenAddr,omitempty"`
	GuestDir       string `json:"guestDir,omitempty"`
	HostDir        string `json:"hostDir,omitempty"`
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
	// general fields
	IsContainerStorage bool   `json:"isContainerStorage"`
	FsType             string `json:"fsType,omitempty"`
	UUID               string `json:"UUID,omitempty"`
	Path               string `json:"path,omitempty"`
	MountTo            string `json:"mountTo,omitempty"`
	SizeInMib          uint64 `json:"sizeInMIB,omitempty"`
}

type PodmanInfo struct {
	// Forward the PodmanAPIUnixSockLocalForward on the host to the Podman API service in
	// the guest which tcp://GuestPodmanAPIListenedIP:GuestPodmanAPIListenedPort
	PodmanAPIUnixSockLocalForward string `json:"podmanAPIUnixSockLocalForward,omitempty"`
	GuestPodmanAPIListenedIP      string `json:"GuestPodmanAPIListenedIP,omitempty"`
	GuestPodmanAPIListenedPort    uint16 `json:"GuestPodmanAPIListenedPort,omitempty"`
}

type GuestAgentCfg struct {
	// ShellCode a mount command to run before guest-agent, because we need to mount the ignition folder to the guest
	ShellCode string `json:"ShellCode,omitempty"`

	// GuestAgentPath the guest-agent is pre-written into the ignition folder and mounted to the guest as a virtiofs, then called and executed by the init program.
	GuestAgentPath string `json:"guestAgentPath,omitempty"`

	Workdir   string `json:"workdir,omitempty"`   // Workdir the working directory for guest-agent
	TargetBin string `json:"targetBin,omitempty"` // TargetBin is the binary to run by guest-agent.

	TargetBinArgs []string `json:"targetBinArgs,omitempty"` // TargetBinArgs is the arguments to pass to the target binary.

	// Env is the environment variables to set for the guest-agent process and target binary, in the form of KEY=VALUE.
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
