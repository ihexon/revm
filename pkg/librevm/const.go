//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

// EventKind identifies a VM lifecycle event.
type EventKind string

const (
	EventStopping EventKind = "stopping"
	EventStopped  EventKind = "stopped"
	EventError    EventKind = "error"
	EventSuccess  EventKind = "success"
	EventExit     EventKind = "exit"

	// Service startup phases.
	EventNetworkStarting     EventKind = "network_starting"
	EventIgnitionStarting    EventKind = "ignition_starting"
	EventManagementStarting  EventKind = "management_starting"
	EventPodmanProxyStarting EventKind = "podman_proxy_starting"

	// Readiness milestones.
	EventNetworkReady EventKind = "network_ready"
	EventSSHReady     EventKind = "ssh_ready"
	EventPodmanReady  EventKind = "podman_ready"

	// VM process lifecycle.
	EventVMStarting EventKind = "vm_starting"
)
