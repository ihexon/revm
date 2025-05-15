package vmconfig

// VMConfig Static virtual machine configuration.
type VMConfig struct {
	MemoryInMB int32
	Cpus       int8
	RootFS     string
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	TargetBin     string
	TargetBinArgs []string
	Env           []string
}
