package event

type StageName string
type EvtName string

const (
	Init StageName = "init"
	Run  StageName = "run"

	Success                  EvtName = "Success"
	Ready                    EvtName = "Ready"
	Exit                     EvtName = "Exit"
	ConfigureVirtualMachine  EvtName = "ConfigureVirtualMachine"
	StartVirtualNetwork      EvtName = "StartVirtualNetwork"
	StartManagementAPIServer EvtName = "StartManagementAPIServer"
	StartGuestCfgServer      EvtName = "StartGuestCfgServer"
	ExtractRootfs            EvtName = "ExtractRootfs"
	Error                    EvtName = "Error"
)
