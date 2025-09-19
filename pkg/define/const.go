package define

const (
	VMConfigFile     = "vmconfig.json"
	HostDNSInGVProxy = "host.containers.internal"
)

const (
	GuestLinuxUtilsBinDir = "/3rd/linux/bin/"
)

const (
	DefaultWorkDir     = "/"
	ThirdPartDirPrefix = "3rd"
	BoostrapFileName   = "bootstrap"

	IgnServerSocketName    = "ignition.sock"
	GvProxyControlEndPoint = "gvpctl.sock"
	GvProxyNetworkEndpoint = "gvpnet.sock"
	DropBearRuntimeDir     = "/run/dropbear"
	DropBearKeyFile        = DropBearRuntimeDir + "/key"
	DropBearPidFile        = DropBearRuntimeDir + "/dropbear.pid"

	SSHKeyPair = "ssh_keypair"

	LockFile = ".lock"

	DefaultPATHInBootstrap = "PATH=" + GuestLinuxUtilsBinDir + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	DefaultPodmanAPIUnixSocksInHost = "/tmp/docker_api.sock"

	DefaultCreateDiskSizeInGB = 200
	Ext4                      = "ext4"

	ContainerStorageMountPoint    = "/var/lib/containers"
	DefaultDataDiskMountDirPrefix = "/var/tmp/mnt"

	DefaultGuestUser                  = "root"
	DefaultGuestSSHListenAddr         = "0.0.0.0"
	PodmanDefaultListenTcpAddrInGuest = "tcp://0.0.0.0:25883"
	DefaultGuestAddr                  = "192.168.127.2"
	DefaultVSockPort                  = 1984

	// RestAPI const var
	RestAPIPodmanReadyURL   = "/ready/podman"
	RestAPISSHReadyURL      = "/ready/sshd"
	RestAPI3rdFileServerURL = "/fileserver/"
	RestAPIVMConfigURL      = "/vmconfig"
)

const (
	FlagDockerMode = "docker-mode"
	FlagRootfsMode = "rootfs-mode"
	FlagKernelMode = "kernel-mode"

	FlagVerbose              = "verbose"
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
	FlagKernel               = "kernel"
	FlagInitrd               = "initrd"
	FlagKernelCmdline        = "kernel-cmdline"
)

type RunMode int

const (
	DockerMode RunMode = iota
	RootFsMode
	KernelMode
)

func (m RunMode) String() string {
	switch m {
	case DockerMode:
		return "docker"
	case RootFsMode:
		return "rootfs"
	case KernelMode:
		return "kernel"
	default:
		return "unknown"
	}
}
