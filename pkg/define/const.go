package define

type (
	RunMode int
)

const (
	VMConfigFileName = "vmconfig.json"
	HostDNSInGVProxy = "host.containers.internal"

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

	DefaultVSockPort          = 25882

	GuestSSHServerPort = 25883
	GuestPodmanAPIPort = 25884
	LocalHost = "127.0.0.1"

	SSHLocalForwardListenPort = 6123

	RestAPIVMConfigURL = "/vmconfig"
	IgnitionVirtioFsTag = "4C3525BF-5FB4-4EF0-8FE6-6F2E9A246221"

	IgnitionFsMountDir = "/dev/shm/ignition"
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
	FlagSaveLogTo        = "save-logs"

	ContainerDiskUUID = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"
	UserDataDiskUUID  = "254879c7-7107-4267-a2c6-d25e27a5358d"
)

const (
	EnvLogLevel = "LOG_LEVEL"
)

const (
	ContainerMode RunMode = iota
	RootFsMode

	OOMOLStudioMode
)

func (m RunMode) String() string {
	switch m {
	case ContainerMode:
		return "container"
	case RootFsMode:
		return "rootfs"
	case OOMOLStudioMode:
		return "oomol-studio"
	default:
		return "unknown"
	}
}

// OVM-specific configuration

type OVMRawDiskType string

const (
	OVMUserDataRawDisk             OVMRawDiskType = "userDataRawDisk"
	OVMUserContainerStorageRawDisk OVMRawDiskType = "userContainerStorageRawDisk"

	FlagWorkspace = "workspace"

	// FlagBoot is a rootfs tar archive that automatically
	// extracts to the specified directory as a bootable rootfs
	FlagBoot = "boot"

	// FlagBootVersion is the version of the rootfs, used to force an update of the rootfs,
	// although it is not frequently used.
	FlagBootVersion = "boot-version"

	// FlagDataVersion is no need anymore, the purpose of keeping it is solely for compatibility with ovm-js
	FlagDataVersion = "data-version"

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
