package protocol

import "linuxvm/pkg/execution"

// Exec v2 uses four independent, flow-controlled Yamux streams: control,
// stdin, stdout and stderr. EOF on stdin is a write-side half-close only.
const ExecVersion = 2

type ExecReady struct {
	Version    int    `json:"version"`
	InstanceID string `json:"instance_id"`
}

// The execution package owns the wire types to keep the runner usable without
// the VM or HTTP packages.
type ExecRequest = execution.Request
