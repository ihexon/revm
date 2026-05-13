package protocol

const AttachSpecVersion = 1

// AttachSpec is the host-side contract for reconnecting to a running VM.
// It intentionally contains only connection data needed by attach clients.
type AttachSpec struct {
	SchemaVersion            int    `json:"schemaVersion"`
	User                     string `json:"user,omitempty"`
	PrivateKeyFile           string `json:"privateKeyFile,omitempty"`
	UseGVProxyTunnel         bool   `json:"useGVProxyTunnel,omitempty"`
	GVPCtlAddr               string `json:"gvpCtlAddr,omitempty"`
	GuestSSHServerListenAddr string `json:"guestSSHServerListenAddr,omitempty"`
	GuestTunnelHost          string `json:"guestTunnelHost,omitempty"`
}
