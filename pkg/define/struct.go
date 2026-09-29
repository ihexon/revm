package define

// MachineSpec contains serializable VM specification data.
type MachineSpec struct {
	WorkspaceDir string `json:"workspaceDir,omitempty"`

	MemoryInMB uint64 `json:"memoryInMB,omitempty"`
	Cpus       uint8  `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// Storage is the complete host-to-guest storage plan. It is prepared before
	// the VMM is built and translated to libkrun devices by the backend.
	Storage StorageSpec `json:"storage,omitempty"`
	// GVproxy control endpoint
	GVPCtlAddr string `json:"GVPCtlAddr,omitempty"`
	// GVPVNetAddr is the unixgram virtio-net backend served by gvproxy.
	GVPVNetAddr string `json:"GVPVNetAddr,omitempty"`
	// GVPNotifyAddr receives gvproxy lifecycle notifications.
	GVPNotifyAddr string `json:"GVPNotifyAddr,omitempty"`

	VirtualNetworkMode VNetMode `json:"virtualNetworkMode,omitempty"`

	LogFile           string            `json:"logFile,omitempty"`
	SSHInfo           SSHInfo           `json:"sshInfo,omitempty"`
	PodmanInfo        PodmanInfo        `json:"podmanInfo,omitempty"` // 仅仅在 docker mode 下有意义
	PortForwards      []PortForward     `json:"portForwards,omitempty"`
	VMCtlAddr         string            `json:"vmCtlAddr,omitempty"`
	RunMode           string            `json:"runMode,omitempty"`
	IgnitionServerCfg IgnitionServerCfg `json:"ignitionServerCfg,omitempty"`
	GuestControlAddr  string            `json:"guestControlAddr,omitempty"`
	GuestAgentCfg     GuestAgentCfg     `json:"guestAgentCfg,omitempty"`
	Cmdline           Cmdline           `json:"cmdline,omitempty"` // 仅仅在 rootfs mode 有意义
	ProxySetting      ProxySetting      `json:"systemProxy,omitempty"`

	TTY bool `json:"TTY"`
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

type StorageSpec struct {
	Blocks   []BlockDeviceSpec `json:"blocks,omitempty"`
	VirtioFS []VirtioFSSpec    `json:"virtiofs,omitempty"`
}

type VirtioFSSpec struct {
	ReadOnly      bool   `json:"readOnly"`
	Source        string `json:"source"`
	Tag           string `json:"tag"`
	Target        string `json:"target"`
	Type          string `json:"type"`
	Opts          string `json:"opts"`
	UUID          string `json:"uuid"`
	DAXWindowSize uint64 `json:"daxWindowSize,omitempty"`
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

// BlockDeviceSpec represents one prepared block device and its guest mount.
type BlockDeviceSpec struct {
	ID         string          `json:"id,omitempty"`
	Path       string          `json:"path,omitempty"`
	Format     uint32          `json:"format,omitempty"`
	ReadOnly   bool            `json:"readOnly,omitempty"`
	DirectIO   bool            `json:"directIO,omitempty"`
	SyncMode   uint32          `json:"syncMode,omitempty"`
	GuestMount *BlockMountSpec `json:"guestMount,omitempty"`
}

type BlockMountSpec struct {
	FsType   string `json:"fsType,omitempty"`
	UUID     string `json:"uuid,omitempty"`
	Target   string `json:"target,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
	Opts     string `json:"opts,omitempty"`
}

const (
	DiskFormatRaw   uint32 = 0
	DiskFormatQCOW2 uint32 = 1
	DiskFormatVMDK  uint32 = 2
	SyncModeNone    uint32 = 0
	SyncModeRelaxed uint32 = 1
	SyncModeFull    uint32 = 2
)

type PodmanInfo struct {
	// HOST
	HostPodmanProxyAddr string `json:"hostPodmanProxyAddr,omitempty"`

	// GUEST
	GuestPodmanAPIListenAddr string   `json:"guestPodmanAPIListenAddr,omitempty"`
	GuestPodmanRunWithEnvs   []string `json:"guestPodmanRunWithEnvs,omitempty"`
}

type PortForward struct {
	Protocol  string `json:"protocol,omitempty"`
	HostIP    string `json:"hostIP,omitempty"`
	HostPort  uint16 `json:"hostPort,omitempty"`
	GuestIP   string `json:"guestIP,omitempty"`
	GuestPort uint16 `json:"guestPort,omitempty"`
}

type PortMapping struct {
	Protocol string `json:"protocol,omitempty"`
	Local    string `json:"local,omitempty"`
	Remote   string `json:"remote,omitempty"`
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
