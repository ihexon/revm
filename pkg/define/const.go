package define

const (
	VMConfigFile    = "vmconfig.json"
	DefaultRestAddr = "127.0.0.1:15731"

	HostDNSInGVProxy = "host.containers.internal"
)

const (
	DefaultWorkDir  = "/"
	BootstrapBinary = "bootstrap"

	GvProxyControlEndPoint = "gvpctl.sock"
	GvProxyNetworkEndpoint = "gvpnet.sock"
	DropBearRuntimeDir     = "/run/dropbear"
	DropBearKeyFile        = DropBearRuntimeDir + "/key"
	DropBearPidFile        = DropBearRuntimeDir + "/dropbear.pid"

	SSHKeyPair = "ssh_keypair"

	DefaultGuestUser          = "root"
	DefaultGuestAddr          = "192.168.127.2"
	DefaultGuestSSHListenAddr = DefaultGuestAddr

	LockFile = ".lock"

	RunDockerEngineMode    = "dockerEngineMode"
	RunUserCommandLineMode = "userCommandLineMode"

	DefaultPATH = "PATH=/3rd:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	DefaultPodmanAPIUnixSocksInHost = "/tmp/docker_api.sock"

	DefaultCreateDiskSizeInGB = 200
	DiskFormat                = "ext4"

	ContainerStorageMountPoint        = "/var/lib/containers"
	DefaultDataDiskMountDirPrefix     = "/var/tmp/mnt"
	PodmanDefaultListenTcpAddrInGuest = "tcp://" + DefaultGuestAddr + ":25883"
)

const (
	FlagVerbose        = "verbose"
	FlagDockerMode     = "docker-mode"
	FlagRootfsMode     = "rootfs-mode"
	FlagListenUnixFile = "listen-unix"

	FlagDiskDisk             = "data-disk"
	FlagContainerDataStorage = "data-storage"

	FlagMount            = "mount"
	FlagRootfs           = "rootfs"
	FlagUsingSystemProxy = "system-proxy"
	FlagMemory           = "memory"
	FlagCpus             = "cpus"
	FlagPTY              = "pty"
)
