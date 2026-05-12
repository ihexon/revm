//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

type VMConfigView struct {
	RunMode     string       `json:"runMode,omitempty"`
	NetworkMode string       `json:"networkMode,omitempty"`
	Resources   ResourceView `json:"resources"`
	Endpoints   EndpointView `json:"endpoints"`
	TTY         bool         `json:"tty"`
	Mounts      []MountView  `json:"mounts,omitempty"`
	Disks       []DiskView   `json:"disks,omitempty"`
}

type ResourceView struct {
	MemoryInMB uint64 `json:"memoryInMB,omitempty"`
	CPUs       uint8  `json:"cpus,omitempty"`
}

type EndpointView struct {
	ManagementAPI string `json:"managementAPI,omitempty"`
	PodmanAPI     string `json:"podmanAPI,omitempty"`
	SSH           string `json:"ssh,omitempty"`
}

type MountView struct {
	ReadOnly bool   `json:"readOnly"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target,omitempty"`
	Type     string `json:"type,omitempty"`
}

type DiskView struct {
	UUID    string `json:"uuid,omitempty"`
	MountTo string `json:"mountTo,omitempty"`
	FsType  string `json:"fsType,omitempty"`
}
