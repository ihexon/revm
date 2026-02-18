package event

type StageName string
type EvtName string

const (
	Run    StageName = "run"
	Docker StageName = "docker"

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
