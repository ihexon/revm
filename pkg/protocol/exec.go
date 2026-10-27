package protocol

// ExecReady identifies the guest reached by the next execution connection.
type ExecReady struct {
	Version    int    `json:"version"`
	InstanceID string `json:"instance_id"`
}
