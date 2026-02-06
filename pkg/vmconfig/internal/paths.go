//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package internal

import (
	"path/filepath"
)

// PathManager handles all workspace-related path calculations.
// It centralizes path generation logic that was previously scattered in VMConfig methods.
type PathManager struct {
	workspacePath string
}

// NewPathManager creates a new PathManager for the given workspace path.
func NewPathManager(workspacePath string) *PathManager {
	return &PathManager{workspacePath: workspacePath}
}

// GetSocksPath returns the path for a socket file within the workspace.
func (p *PathManager) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(p.workspacePath, "socks", name))
}

// GetPodmanListenAddr returns the path to the Podman API socket.
func (p *PathManager) GetPodmanListenAddr() string {
	return p.GetSocksPath("podman-api.sock")
}

// GetVNetListenAddr returns the path to the virtual network socket.
func (p *PathManager) GetVNetListenAddr() string {
	return p.GetSocksPath("vnet.sock")
}

// GetGVPCtlAddr returns the path to the gvisor-tap-vsock control socket.
func (p *PathManager) GetGVPCtlAddr() string {
	return p.GetSocksPath("gvpctl.sock")
}

// GetVMCtlAddr returns the path to the VM control API socket.
func (p *PathManager) GetVMCtlAddr() string {
	return p.GetSocksPath("vmctl.sock")
}

// GetIgnAddr returns the path to the ignition server socket.
func (p *PathManager) GetIgnAddr() string {
	return p.GetSocksPath("ign.sock")
}

// GetSSHPrivateKeyFile returns the path to the SSH private key file.
func (p *PathManager) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(p.workspacePath, "ssh", "key"))
}

// GetLogsDir returns the path to the logs directory.
func (p *PathManager) GetLogsDir() string {
	return filepath.Join(p.workspacePath, "logs")
}

// GetRootfsDir returns the path to the rootfs directory.
func (p *PathManager) GetRootfsDir() string {
	return filepath.Join(p.workspacePath, "rootfs")
}

// GetContainerStorageDiskPath returns the path to the container storage raw disk.
func (p *PathManager) GetContainerStorageDiskPath() string {
	return filepath.Join(p.workspacePath, "raw-disk", "container-storage.ext4")
}
