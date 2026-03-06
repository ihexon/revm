package define

const (
	FlagDockerMode = "docker"
	FlagChroot     = "chroot"
	FlagAttachMode = "attach"
	FlagClean      = "clean"

	FlagLogLevel         = "log-level"
	FlagLogTo            = "log-to"
	FlagCPUS             = "cpus"
	FlagRawDisk          = "raw-disk"
	FlagMount            = "mount"
	FlagRootfs           = "rootfs"
	FlagUsingSystemProxy = "system-proxy"
	FlagWorkDir          = "workdir"
	FlagMemoryInMB       = "memory"
	FlagPTY              = "pty"
	FlagEnvs             = "envs"
	FlagReportURL        = "report-url"
	FlagVNetworkType     = "network"
	FlagSessionID        = "id"
	FlagContainerDisk    = "container-disk"
	FlagPodmanProxyAPI   = "podman-proxy-api"
	FlagManageAPI        = "manage-api"

	ContainerDiskUUID = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"

	GuestLogConsolePort = "guest-logs"
	GuestTTYConsoleName = "default-tty-console"
)

const (
	FlagOVMInit  = "init"
	FlagOVMStart = "start"

	FlagOVMBoot        = "boot"
	FlagOVMBootVersion = "boot-version"

	FlagOVMContainerDiskVersion = "data-version"

	FlagOVMPPID = "ppid"

	FlagOVMVolume = "volume"

	FlagOVMName = "name"

	FlagOVMLogLevel   = "log-level"
	FlagOVMWorkspace  = "workspace"
	FlagOVMReportURL  = "report-url"
	FlagOVMCPUS       = "cpus"
	FlagOVMMemoryInMB = "memory"

	SubCmdOVMInit  = "init"
	SubCmdOVMStart = "start"

	OVMContainerStorageMountPoint = "/var/lib/containers"
	OVMContainerStorageDiskUUID   = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"
	OVMUserDataStorageDiskUUID    = "254879c7-7107-4267-a2c6-d25e27a5358d"
)
