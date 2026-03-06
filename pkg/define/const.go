package define

import (
	"fmt"
	"time"
)

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

type VNetMode string

const (
	GVISOR VNetMode = "gvisor"
	TSI    VNetMode = "tsi"
)

const (
	BuiltinBusybox          = "/.bin/busybox"
	GuestAgentPathInGuest   = "/.bin/guest-agent"
	GuestHiddenBinDir       = "/.bin"
	VMConfigFilePathInGuest = "/vmconfig.json"
	HostDomainInGVPNet      = "host.containers.internal"

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

	LocalHost = "127.0.0.1"

	SSHLocalForwardListenPort = 6123

	ServiceNamePodman       = "podman"
	ServiceNameSSH          = "ssh"
	ServiceNameGuestNetwork = "guest-network"
)

const (
	RestAPIVMConfigURL = "/vmconfig"
)

const (
	EnvLogLevel = "LOG_LEVEL"

	DefaultTimeTicker   = 100 * time.Millisecond
	DefaultProbeTimeout = 60 * time.Second
)

var (
	ErrStopChTrigger     = fmt.Errorf("machine stop triggered")
	ErrParentProcessExit = fmt.Errorf("parent process exit")
	ErrSigTerm           = fmt.Errorf("received SIGTERM/SIGINT, shutting down")
)
