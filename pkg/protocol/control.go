package protocol

// GuestControlRequest is the versioned host-to-guest command request.
// Commands are sent as argv, never as shell-escaped strings.
type GuestControlRequest struct {
	SchemaVersion int      `json:"schemaVersion"`
	Bin           string   `json:"bin"`
	Args          []string `json:"args,omitempty"`
	Env           []string `json:"env,omitempty"`
	WorkDir       string   `json:"workDir,omitempty"`
	Stdin         []byte   `json:"stdin,omitempty"`
}

const GuestControlVersion = 1

type GuestControlFrame struct {
	Type     string `json:"type"`
	Data     []byte `json:"data,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

const (
	GuestControlStdout = "stdout"
	GuestControlStderr = "stderr"
	GuestControlExit   = "exit"
	GuestControlError  = "error"
)
