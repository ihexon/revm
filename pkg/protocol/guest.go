package protocol

const GuestSpecVersion = 7

// GuestSpec is the versioned host-to-guest boot contract.
// Keep this type limited to fields consumed inside the guest-agent.
type GuestSpec struct {
	SchemaVersion int          `json:"schemaVersion"`
	Network       string       `json:"network"`
	ResolvConf    string       `json:"resolvConf,omitempty"`
	RunMode       string       `json:"runMode,omitempty"`
	Storage       GuestStorage `json:"storage,omitempty"`
	Podman        GuestPodman  `json:"podman,omitempty"`
}

type GuestStorage struct {
	VirtioFS []GuestMount    `json:"virtiofs,omitempty"`
	Blocks   []GuestBlockDev `json:"blocks,omitempty"`
}

type GuestMount struct {
	ReadOnly bool   `json:"readOnly"`
	Tag      string `json:"tag,omitempty"`
	Target   string `json:"target"`
}

type GuestBlockDev struct {
	FsType   string `json:"fsType,omitempty"`
	UUID     string `json:"UUID,omitempty"`
	MountTo  string `json:"mountTo,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

type GuestPodman struct {
	GuestPodmanRunWithEnvs []string `json:"guestPodmanRunWithEnvs,omitempty"`
}
