//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import "path/filepath"

// machinePathManager handles all workspace-relative path calculations.
type machinePathManager struct {
	workspaceDir string
	rootfsDir    string
}

func newMachinePathManager(workspaceDir string) *machinePathManager {
	return &machinePathManager{workspaceDir: workspaceDir}
}

func (p *machinePathManager) GetSocketFile(name string) string {
	return filepath.Clean(filepath.Join(p.workspaceDir, "socks", name))
}

func (p *machinePathManager) GetPodmanSocketFile() string {
	return p.GetSocketFile("podman-api.sock")
}

func (p *machinePathManager) GetPodmanTransportSocketFile() string {
	return p.GetSocketFile("podman-transport.sock")
}

func (p *machinePathManager) GetVNetSocketFile() string {
	return p.GetSocketFile("vnet.sock")
}

func (p *machinePathManager) GetGVPCtlSocketFile() string {
	return p.GetSocketFile("gvpctl.sock")
}

func (p *machinePathManager) GetGVPNotifySocketFile() string {
	return p.GetSocketFile("gvpnotify.sock")
}

func (p *machinePathManager) GetVMCtlSocketFile() string {
	return p.GetSocketFile("vmctl.sock")
}

func (p *machinePathManager) GetGuestControlSocketFile() string {
	return p.GetSocketFile("guest-control.sock")
}

func (p *machinePathManager) GetRootfsDir() string { return p.rootfsDir }
