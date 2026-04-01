//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import "path/filepath"

// machinePathManager handles all workspace-relative path calculations.
type machinePathManager struct {
	workspaceDir string
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

func (p *machinePathManager) GetVNetSocketFile() string {
	return p.GetSocketFile("vnet.sock")
}

func (p *machinePathManager) GetGVPCtlSocketFile() string {
	return p.GetSocketFile("gvpctl.sock")
}

func (p *machinePathManager) GetVMCtlSocketFile() string {
	return p.GetSocketFile("vmctl.sock")
}

func (p *machinePathManager) GetIgnSocketFile() string {
	return p.GetSocketFile("ign.sock")
}

// GetSSHKeyFilePath returns the path to the SSH key pair file
func (p *machinePathManager) GetSSHKeyFilePath() string {
	return filepath.Clean(filepath.Join(p.workspaceDir, "ssh", "ssh-key"))
}

func (p *machinePathManager) GetLogsDir() string {
	return filepath.Join(p.workspaceDir, "logs")
}

func (p *machinePathManager) GetRootfsDir() string {
	return filepath.Join(p.workspaceDir, "rootfs")
}

func (p *machinePathManager) GetBuiltInContainerStorageDiskFile() string {
	return filepath.Join(p.workspaceDir, "raw-disk", "container-storage.ext4")
}
