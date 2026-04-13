package define

import (
	"encoding/json"
)

// MachineSpec contains serializable VM specification data.
type MachineSpec struct {
	WorkspaceDir string `json:"workspaceDir,omitempty"`

	MemoryInMB uint64 `json:"memoryInMB,omitempty"`
	Cpus       uint8  `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX and automount by guest-agent process
	BlkDevs []BlkDev `json:"blkDevs,omitempty"`
	// GVproxy control endpoint
	GVPCtlAddr string `json:"GVPCtlAddr,omitempty"`
	// GVPVNetAddr is the network stack backend to use. which provided
	// by gvproxy
	GVPVNetAddr string `json:"GVPVNetAddr,omitempty"`

	VirtualNetworkMode VNetMode `json:"virtualNetworkMode,omitempty"`

	LogFile           string            `json:"logFile,omitempty"`
	Mounts            []Mount           `json:"mounts,omitempty"`
	SSHInfo           SSHInfo           `json:"sshInfo,omitempty"`
	PodmanInfo        PodmanInfo        `json:"podmanInfo,omitempty"` // 仅仅在 docker mode 下有意义
	VMCtlAddr         string            `json:"vmCtlAddr,omitempty"`
	RunMode           string            `json:"runMode,omitempty"`
	IgnitionServerCfg IgnitionServerCfg `json:"ignitionServerCfg,omitempty"`
	GuestAgentCfg     GuestAgentCfg     `json:"guestAgentCfg,omitempty"`
	Cmdline           Cmdline           `json:"cmdline,omitempty"` // 仅仅在 rootfs mode 有意义
	ProxySetting      ProxySetting      `json:"systemProxy,omitempty"`

	TTY bool `json:"TTY"`
}

// MachineRuntime contains non-serializable runtime state.
type MachineRuntime struct {
	Readiness *Readiness `json:"-"`
}

func NewMachineRuntime() *MachineRuntime {
	return &MachineRuntime{
		Readiness: NewReadiness(),
	}
}

// Machine combines serializable spec and runtime state.
type Machine struct {
	MachineSpec
	*MachineRuntime `json:"-"`
}

func (m *Machine) EnsureRuntime() {
	if m == nil {
		return
	}
	if m.MachineRuntime == nil {
		m.MachineRuntime = NewMachineRuntime()
	}
}

func (m *Machine) UnmarshalJSON(data []byte) error {
	type machineAlias MachineSpec
	var spec machineAlias
	if err := json.Unmarshal(data, &spec); err != nil {
		return err
	}

	m.MachineSpec = MachineSpec(spec)
	m.EnsureRuntime()
	return nil
}

func (m Machine) MarshalJSON() ([]byte, error) {
	type machineAlias MachineSpec
	return json.Marshal(machineAlias(m.MachineSpec))
}

const (
	XattrDiskVersionKey = "user.vm.rawdisk.version"
)

type Cmdline struct {
	Envs    []string `json:"envs,omitempty"`
	Bin     string   `json:"bin,omitempty"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
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
	// HOST
	HostSSHPrivateKeyFile  string `json:"hostSSHKeyFile,omitempty"`
	HostSSHPublicKey       string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey      string `json:"sshPrivateKey,omitempty"`
	HostSSHProxyListenAddr string `json:"hostSSHProxyListenAddr,omitempty"`

	// GUEST
	GuestSSHServerListenAddr string `json:"guestSSHServerListenAddr,omitempty"`
	GuestSSHPrivateKeyFile   string `json:"guestSSHPrivateKeyFile,omitempty"`
	GuestSSHAuthorizedKeys   string `json:"guestSSHAuthorizedKeys,omitempty"`
	GuestSSHPidFile          string `json:"guestSSHPidFile,omitempty"`
}

type ProxySetting struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	Use        bool   `json:"use,omitempty"`
}

type IgnitionServerCfg struct {
	ListenSockAddr string `json:"ListenSockAddr,omitempty"`
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
	// HOST
	HostPodmanProxyAddr string `json:"hostPodmanProxyAddr,omitempty"`

	// GUEST
	GuestPodmanAPIListenAddr string   `json:"guestPodmanAPIListenAddr,omitempty"`
	GuestPodmanRunWithEnvs   []string `json:"guestPodmanRunWithEnvs,omitempty"`
}

type GuestAgentCfg struct {
	Workdir string   `json:"workdir,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type GuestSignalName string

const (
	GuestSignalInterrupt  GuestSignalName = "interrupt"
	GuestSignalTerminated GuestSignalName = "terminated"
	GuestSignalQuit       GuestSignalName = "quit"
)

type GuestSignal struct {
	SignalName GuestSignalName `json:"signalName,omitempty"`
}
