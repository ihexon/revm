//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import "time"

// EventKind identifies a VM lifecycle event.
type EventKind string

const (
	EventConfiguring         EventKind = "configuring"
	EventNetworkStarting     EventKind = "network_starting"
	EventIgnitionStarting    EventKind = "ignition_starting"
	EventManagementStarting  EventKind = "management_starting"
	EventPodmanProxyStarting EventKind = "podman_proxy_starting"
	EventNetworkReady        EventKind = "network_ready"
	EventVMStarting          EventKind = "vm_starting"
	EventSSHReady            EventKind = "ssh_ready"
	EventPodmanReady         EventKind = "podman_ready"
	EventStopping            EventKind = "stopping"
	EventStopped             EventKind = "stopped"
	EventError               EventKind = "error"
)

// Event represents a single VM lifecycle event.
type Event struct {
	Kind    EventKind `json:"kind"`
	Message string    `json:"message,omitempty"`
	VMName  string    `json:"vmName,omitempty"`
	Seq     uint64    `json:"seq,omitempty"`
	Time    time.Time `json:"time"`
}
