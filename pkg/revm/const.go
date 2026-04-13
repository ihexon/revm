//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

// EventKind identifies a VM lifecycle event.
type EventKind string

const (
	EventStopping EventKind = "stopping"

	EventManagementAPIStarting EventKind = "start_vm_management_api"
	EventHostNetworkStack      EventKind = "host_network_stack"
	EventIgnitionService       EventKind = "ignition_service"
	EventVirtualMachineBooting EventKind = "virtual_machine_booting"

	// Readiness milestones.
	EventNetworkReady EventKind = "network_ready"
	EventSSHReady     EventKind = "ssh_ready"
	EventPodmanReady  EventKind = "podman_ready"
)
