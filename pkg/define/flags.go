package define

const (
	FlagDockerMode = "dockerd"
	FlagChroot     = "chroot"
	FlagAttachMode = "attach"

	FlagLogLevel                = "log-level"
	FlagLogTo                   = "log-to"
	FlagCPUS                    = "cpus"
	FlagRawDisk                 = "raw-disk"
	FlagMount                   = "mount"
	FlagRootfs                  = "rootfs"
	FlagUsingSystemProxy        = "system-proxy"
	FlagWorkDir                 = "workdir"
	FlagMemoryInMB              = "memory"
	FlagPTY                     = "pty"
	FlagEnvs                    = "envs"
	FlagVNetworkType            = "network"
	FlagSessionID               = "id"
	FlagContainerDisk           = "container-disk"
	FlagPodmanProxyAPIFile      = "podman-api"
	FlagManageAPIFile           = "manage-api"
	FlagExportSSHKeyPrivateFile = "ssh-key"
	FlagReportEvents            = "report-events"

	ContainerDiskUUID = "162cf68f-93c7-49ad-be53-45ed0e9fe42b"

	GuestLogConsolePort    = "guest-log"
	GuestSignalConsolePort = "guest-signal"
	GuestTTYConsoleName    = "default-tty-console"

	KrunStdinPortName  = "krun-stdin"
	KrunStdoutPortName = "krun-stdout"
	KrunStderrPortName = "krun-stderr"
)
