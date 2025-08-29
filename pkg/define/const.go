package define

const (
	VMConfigFile    = "vmconfig.json"
	DefaultRestAddr = "127.0.0.1:15731"
)

const (
	DefalutWorkDir  = "/"
	PrefixDir3rd    = "3rd"
	DropBearBinary  = PrefixDir3rd + "/dropbear"
	BootstrapBinary = PrefixDir3rd + "/bootstrap"

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
)
