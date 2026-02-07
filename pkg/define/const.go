package define

const (
	ContainerMode RunMode = iota
	RootFsMode

	// OVMode As the underlying virtual machine running mode for Oomol Studio, this mode bundles many default business logic
	OVMode
)

type RunMode int32

func (m RunMode) String() string {
	switch m {
	case ContainerMode:
		return "container"
	case RootFsMode:
		return "rootfs"
	case OVMode:
		return "oomol-studio"
	default:
		return "unknown"
	}
}

type VNetMode uint32

const (
	GVISOR VNetMode = iota
	TSI
	UNKNOWN
)

func (v VNetMode) String() string {
	switch v {
	case GVISOR:
		return "GVISOR"
	case TSI:
		return "TSI"
	default:
		return "unknown"
	}
}

func String2NetworkMode(s string) VNetMode {
	switch s {
	case GVISOR.String():
		return GVISOR
	case TSI.String():
		return TSI
	default:
		return UNKNOWN
	}
}

const (
	GuestAgentPathInGuest   = "/.bin/guest-agent"
	GuestHiddenBinDir       = "/.bin"
	VMConfigFilePathInGuest = "/vmconfig.json"
	HostDomainInGVPNet      = "host.containers.internal"

	RootfsDirName  = "rootfs"
	LibexecDirName = "libexec"

	SSHPrivateKeyFileName = "private.key"
	SSHPublicKeyFileName  = "public.key"

	DropBearRuntimeDir = "/run/dropbear"

	DropBearPrivateKeyPath = DropBearRuntimeDir + "/" + SSHPrivateKeyFileName

	DropBearPidFile = DropBearRuntimeDir + "/dropbear.pid"

	ContainerStorageMountPoint = "/var/lib/containers"

	DefaultGuestUser = "root"

	UnspecifiedAddress = "0.0.0.0"
	GuestIP            = "192.168.127.2"

	DefaultVSockPort = 25882

	GuestSSHServerPort = 25883
	GuestPodmanAPIPort = 25884
	LocalHost          = "127.0.0.1"

	SSHLocalForwardListenPort = 6123
)

const (
	RestAPIVMConfigURL = "/vmconfig"
)

const (
	FlagDockerMode = "docker-mode"
	FlagRootfsMode = "rootfs-mode"

	FlagLogLevel         = "log-level"
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
	FlagWorkspace        = "workspace"

	ContainerDiskUUID = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"
	UserDataDiskUUID  = "254879c7-7107-4267-a2c6-d25e27a5358d"

	GuestLogConsolePort = "guest-logs"
)

const (
	EnvLogLevel = "LOG_LEVEL"
)

// OVMode-specific configuration

const (
	FlagInit  = "init"
	FlagStart = "start"

	// FlagBoot is a rootfs tar archive that automatically
	// extracts to the specified directory as a bootable rootfs
	FlagBoot = "boot"

	// FlagBootVersion is the version of the rootfs, used to force an update of the rootfs,
	// although it is not frequently used.
	FlagBootVersion = "boot-version"

	// FlagContainerRAWVersionXATTR controls whether container-disk.ext4 needs to be updated. If the new field value differs from the old one, container-disk.ext4 will be deleted and regenerated.
	// The name "data-version" is not semantically accurate, but it's kept for compatibility reasons. It may be renamed to "container-disk-version" in the future.
	FlagContainerRAWVersionXATTR = "data-version"

	// FlagPPID is no need anymore, the purpose of keeping it is solely for compatibility with ovm-js
	FlagPPID = "ppid"

	// FlagVolume mount host directory to guest
	FlagVolume = "volume"

	// FlagName is not used anymore, the purpose of keeping it is solely for compatibility with ovm-js
	FlagName = "name"

	SubCmdInit  = "init"
	SubCmdStart = "start"

	OVMUserDataDiskMountPoint = "/mnt/user-data"

	OVMContainerStorageMountPoint = ContainerStorageMountPoint

	OVMContainerStorageDiskUUID = ContainerDiskUUID
	OVMUserDataStorageDiskUUID  = UserDataDiskUUID
)
