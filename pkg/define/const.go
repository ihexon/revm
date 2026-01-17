package define

const (
	VMConfigFile     = "vmconfig.json"
	HostDNSInGVProxy = "host.containers.internal"
)

const (
	KernelPrefixDir = "kernel"
)

const (
	DefaultWorkDir = "/"
	RootfsDirName  = "rootfs"
	LibexecDirName = "libexec"

	IgnServerSocketName    = "ignition.sock"
	GvProxyControlEndPoint = "gvpctl.sock"
	GvProxyNetworkEndpoint = "gvpnet.sock"
	DropBearRuntimeDir     = "/run/dropbear"
	DropBearKeyFile        = DropBearRuntimeDir + "/key"
	DropBearPidFile        = DropBearRuntimeDir + "/dropbear.pid"

	SSHKeyPair = "ssh_keypair"

	LockFile = ".lock"

	DefaultPodmanAPIUnixSocksInHost = "/tmp/docker_api.sock"

	DefaultCreateDiskSizeInGB = 200

	ContainerStorageMountPoint = "/var/lib/containers"

	DefaultGuestUser = "root"

	UnspecifiedAddress = "0.0.0.0"
	DefaultGuestAddr   = "192.168.127.2"

	DefaultVSockPort          = 25882
	DefaultGuestSSHDPort      = 25883
	DefaultGuestPodmanAPIPort = 25884

	RestAPIVMConfigURL = "/vmconfig"
)

const (
	FlagDockerMode = "docker-mode"
	FlagRootfsMode = "rootfs-mode"

	FlagLogLevel             = "log-level"
	FlagListenUnixFile       = "listen-unix"
	FlagRestAPIListenAddr    = "rest-api"
	FlagCPUS                 = "cpus"
	FlagDiskDisk             = "data-disk"
	FlagContainerDataStorage = "data-storage"
	FlagMount                = "mount"
	FlagRootfs               = "rootfs"
	FlagUsingSystemProxy     = "system-proxy"
	FlagMemory               = "memory"
	FlagPTY                  = "pty"
	FlagEnvs                 = "envs"
	FlagReportURL            = "report-url"
	FlagSaveLogTo            = "save-logs"
)

type RunMode int

const (
	ContainerMode RunMode = iota
	RootFsMode
)

func (m RunMode) String() string {
	switch m {
	case ContainerMode:
		return "container"
	case RootFsMode:
		return "rootfs"
	default:
		return "unknown"
	}
}
