package event

type StageName string
type EvtName string

const (
	Chroot StageName = "chroot"
	Docker StageName = "docker"
	Clean  StageName = "clean"
	Attach StageName = "attach"

	Success                  EvtName = "Success" // init success, only in ovm
	AllThingsReady           EvtName = "Ready"
	Exit                     EvtName = "Exit"
	ConfigureVirtualMachine  EvtName = "ConfigureVirtualMachine"
	StartVirtualNetwork      EvtName = "StartVirtualNetwork"
	StartManagementAPIServer EvtName = "StartManagementAPIServer"
	StartIgnitionServer      EvtName = "StartIgnitionServer"
	StartPodmanProxyServer   EvtName = "StartPodmanProxyServer"
	StartVirtualMachine      EvtName = "StartVirtualMachine"
	GuestSSHReady            EvtName = "GuestSSHReady"
	GuestPodmanReady         EvtName = "GuestPodmanReady"
	GuestNetworkReady        EvtName = "GuestNetworkReady"
	HostNetworkReady         EvtName = "HostNetworkReady"
	RootfsExtractedReady     EvtName = "RootfsExtractedReady"
	Error                    EvtName = "Error"
)
