package define

const (
	VMConfigFile    = "vmconfig.json"
	DefaultRestAddr = "127.0.0.1:15731"
)

const (
	DefalutWorkDir  = "/"
	BootstrapBinary = "bootstrap"

	GvProxyControlEndPoint = "gvpctl.sock"
	GvProxyNetworkEndpoint = "gvpnet.sock"
	DropBearRuntimeDir     = "/run/dropbear"
	DropBearKeyFile        = DropBearRuntimeDir + "/key"
	DropBearPidFile        = DropBearRuntimeDir + "/dropbear.pid"

	SSHKeyPair = "ssh_keypair"

	DefaultSSHInHost = "127.0.0.1"

	DefaultGuestUser           = "root"
	DefaultGuestSSHPort uint64 = 22
	DefaultGuestSSHAddr        = "192.168.127.2"

	LockFile = ".lock"

	RunDockerEngineMode    = "dockerEngineMode"
	RunUserCommandLineMode = "userCommandLineMode"

	DefaultPATH = "PATH=/3rd:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

	DefaultPodmanAPIUnixSocksInHost = "/tmp/my_docker_api.sock"

	DefaultCreateDiskSizeInGB = 100
	DiskFormat                = "ext4"

	ContainerStorage                = "ContainerStorage"
	GeneralStorage                  = "GeneralStorage"
	DefaultContainerStorageDiskUUID = "783E1C2C-AF2F-47FC-9EB1-0AAD9234D55B"
	ContainerStorageMountPoint      = "/var/lib/containers"
	DefaultDataDiskMountDirPrefix   = "/var/tmp/mnt"
)

const (
	FlagDockerMode = "docker-mode"
	FlagRootfsMode = "rootfs-mode"
	FlagListenUnix = "listen-unix"

	FlagDiskDisk = "data-disk"
	//FlagCreateDataDisk       = "create-disk"
	FlagContainerDataStorage = "data-storage"

	FlagMount            = "mount"
	FlagRootfs           = "rootfs"
	FlagUsingSystemProxy = "system-proxy"
)
