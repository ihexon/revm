package define

import (
	"time"
)

type RunMode string

const (
	ContainerMode RunMode = "container"
	RootFsMode    RunMode = "rootfs"
)

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

	ContainerStorageMountPoint = "/var/lib/containers"

	UnspecifiedAddress = "0.0.0.0"
	GuestIP            = "192.168.127.2"
	GuestCIDR          = "192.168.127.2/24"
	GatewayIP          = "192.168.127.1"
	GuestSubnet        = "192.168.127.0/24"

	GuestControlPort  = 25883
	GuestPodmanPort   = 25884
	GuestPodmanSocket = "/run/revm/podman/podman.sock"
	// libkrun assigns the guest CID 3.
	GuestVSockCID = 3

	LocalHost = "127.0.0.1"
)

const (
	EnvLogLevel = "LOG_LEVEL"

	DefaultTimeTicker = 100 * time.Millisecond
)
