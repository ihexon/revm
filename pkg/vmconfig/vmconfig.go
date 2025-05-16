package vmconfig

// VMConfig Static virtual machine configuration.
type VMConfig struct {
	MemoryInMB int32
	Cpus       int8
	RootFS     string

	// data disk will map into /dev/vdX
	DataDisk string
	// GVproxy control endpoint
	GVproxyEndpoint string
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	TargetBin     string
	TargetBinArgs []string
	Env           []string
}
