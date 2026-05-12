package protocol

const GuestSpecVersion = 1

// GuestSpec is the versioned host-to-guest boot contract.
// Keep this type limited to fields consumed inside the guest-agent.
type GuestSpec struct {
	SchemaVersion int             `json:"schemaVersion"`
	RunMode       string          `json:"runMode,omitempty"`
	NetworkMode   string          `json:"networkMode,omitempty"`
	TTY           bool            `json:"tty,omitempty"`
	Cmdline       GuestCmdline    `json:"cmdline,omitempty"`
	Mounts        []GuestMount    `json:"mounts,omitempty"`
	BlkDevs       []GuestBlockDev `json:"blkDevs,omitempty"`
	SSH           GuestSSH        `json:"ssh,omitempty"`
	Podman        GuestPodman     `json:"podman,omitempty"`
}

type GuestCmdline struct {
	Envs    []string `json:"envs,omitempty"`
	Bin     string   `json:"bin,omitempty"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
}

type GuestMount struct {
	ReadOnly bool   `json:"readOnly"`
	Source   string `json:"source,omitempty"`
	Tag      string `json:"tag,omitempty"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	Opts     string `json:"opts,omitempty"`
	UUID     string `json:"uuid,omitempty"`
}

type GuestBlockDev struct {
	FsType  string `json:"fsType,omitempty"`
	UUID    string `json:"UUID,omitempty"`
	Path    string `json:"path,omitempty"`
	MountTo string `json:"mountTo,omitempty"`
}

type GuestSSH struct {
	HostSSHPublicKey         string `json:"sshPublicKey,omitempty"`
	GuestSSHServerListenAddr string `json:"guestSSHServerListenAddr,omitempty"`
	GuestSSHPrivateKeyFile   string `json:"guestSSHPrivateKeyFile,omitempty"`
	GuestSSHAuthorizedKeys   string `json:"guestSSHAuthorizedKeys,omitempty"`
	GuestSSHPidFile          string `json:"guestSSHPidFile,omitempty"`
}

type GuestPodman struct {
	GuestPodmanAPIListenAddr string   `json:"guestPodmanAPIListenAddr,omitempty"`
	GuestPodmanRunWithEnvs   []string `json:"guestPodmanRunWithEnvs,omitempty"`
}
