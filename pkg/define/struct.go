package define

type VMConfig struct {
	WorkspacePath string `json:"workspacePath,omitempty"`

	MemoryInMB uint64 `json:"memoryInMB,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX and automount by guest-agent process
	BlkDevs []BlkDev `json:"blkDevs,omitempty"`
	// GVproxy control endpoint
	GVPCtlAddr string `json:"GVPCtlAddr,omitempty"`
	// GVPVNetAddr is the network stack backend to use. which provided
	// by gvproxy
	GVPVNetAddr string `json:"GVPVNetAddr,omitempty"`

	VirtualNetworkMode string `json:"virtualNetworkMode,omitempty"`

	LogFilePath       string            `json:"logFilePath,omitempty"`
	Mounts            []Mount           `json:"mounts,omitempty"`
	SSHInfo           SSHInfo           `json:"sshInfo,omitempty"`
	PodmanInfo        PodmanInfo        `json:"podmanInfo,omitempty"` // 仅仅在 docker mode 下有意义
	VMCtlAddress      string            `json:"vmCTLAddress,omitempty"`
	RunMode           string            `json:"runMode,omitempty"`
	IgnitionServerCfg IgnitionServerCfg `json:"ignitionServerCfg,omitempty"`
	GuestAgentCfg     GuestAgentCfg     `json:"guestAgentCfg,omitempty"`
	Cmdline           Cmdline           `json:"cmdline,omitempty"` // 仅仅在 rootfs mode 有意义
	XATTRSRawDisk     map[string]string `json:"XATTRSRawDisk,omitempty"`

	StopCh chan struct{} `json:"-"`
}

const (
	XATTRRawDiskVersionKey = "user.vm.rawdisk.version"
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
	ListenSockAddr string `json:"ListenSockAddr,omitempty"`
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
	PodmanProxyAddr    string   `json:"podmanProxyAddr,omitempty"`
	GuestPodmanAPIIP   string   `json:"GuestPodmanAPIIP,omitempty"`
	GuestPodmanAPIPort uint16   `json:"GuestPodmanAPIPort,omitempty"`
	Envs               []string `json:"envs,omitempty"`
}

type GuestAgentCfg struct {
	Workdir string   `json:"workdir,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

// NeedsGuestNetworkConfig returns true if the guest needs to configure network.
// TSI mode handles networking transparently, so guest doesn't need to configure.
// GVISOR mode requires guest to configure network via tap interface.
func (v *VMConfig) NeedsGuestNetworkConfig() bool {
	return v.VirtualNetworkMode != TSI.String()
}
