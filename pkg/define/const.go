package define

import (
	"time"
)

const (
	ContainerMode RunMode = iota
	RootFsMode
)

type RunMode int32

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

type VNetMode string

const (
	GVISOR VNetMode = "gvisor"
	TSI    VNetMode = "tsi"
)

const (
	GuestAgentPathInGuest   = "/.bin/guest-agent"
	GuestHiddenBinDir       = "/.bin"
	VMConfigFilePathInGuest = "/vmconfig.json"
	HostDomainInGVPNet      = "host.containers.internal"

	ContainerStorageMountPoint  = "/var/lib/containers"
	DefaultContainerDiskVersion = "revm-container-storage-v1"

	DefaultGuestUser = "root"

	UnspecifiedAddress = "0.0.0.0"
	GuestIP            = "192.168.127.2"
	GuestCIDR          = "192.168.127.2/24"
	GatewayIP          = "192.168.127.1"
	GuestSubnet        = "192.168.127.0/24"

	DefaultVSockPort = 25882
	GuestControlPort = 25883
	// libkrun assigns the guest CID 3; CID 2 is the host and is used by the
	// guest agent when it connects back to the ignition server.
	GuestVSockCID = 3

	LocalHost = "127.0.0.1"

	SSHLocalForwardListenPort = 6123
)

const (
	RestAPIVMConfigURL = "/vmconfig"
)

const (
	EnvLogLevel = "LOG_LEVEL"

	DefaultTimeTicker   = 100 * time.Millisecond
	DefaultProbeTimeout = 60 * time.Second
)
