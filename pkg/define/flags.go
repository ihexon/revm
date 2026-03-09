package define

const (
	FlagDockerMode = "docker"
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
	FlagPodmanProxyAPIFile      = "podman-proxy-api-file"
	FlagManageAPIFile           = "manage-api-file"
	FlagSSHKeyDir               = "ssh-key-dir"
	FlagExportSSHKeyPrivateFile = "export-ssh-private-key"
	FlagExportSSHKeyPublicFile  = "export-ssh-public-key"
	FlagReportEvents            = "report-events-to"

	ContainerDiskUUID = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"

	GuestLogConsolePort = "guest-logs"
	GuestTTYConsoleName = "default-tty-console"

	KrunStdinPortName  = "krun-stdin"
	KrunStdoutPortName = "krun-stdout"
	KrunStderrPortName = "krun-stderr"
)

const (
	FlagOVMBoot                 = "boot"
	FlagOVMBootVersion          = "boot-version"
	FlagOVMContainerDiskVersion = "data-version"
	FlagOVMPPID                 = "ppid"
	FlagOVMVolume               = "volume"
	FlagOVMName                 = "name"
	FlagOVMWorkspace            = "workspace"
	FlagOVMReportURL            = "report-url"
)
