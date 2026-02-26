//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import "path/filepath"

// PathManager handles all workspace-relative path calculations.
type PathManager struct {
	workspacePath string
}

// NewPathManager creates a new PathManager for the given workspace path.
func NewPathManager(workspacePath string) *PathManager {
	return &PathManager{workspacePath: workspacePath}
}

func (p *PathManager) GetSocksPath(name string) string {
	return filepath.Clean(filepath.Join(p.workspacePath, "socks", name))
}

func (p *PathManager) GetPodmanListenAddr() string {
	return p.GetSocksPath("podman-api.sock")
}

func (p *PathManager) GetVNetListenAddr() string {
	return p.GetSocksPath("vnet.sock")
}

func (p *PathManager) GetGVPCtlAddr() string {
	return p.GetSocksPath("gvpctl.sock")
}

func (p *PathManager) GetVMCtlAddr() string {
	return p.GetSocksPath("vmctl.sock")
}

func (p *PathManager) GetIgnAddr() string {
	return p.GetSocksPath("ign.sock")
}

func (p *PathManager) GetSSHPrivateKeyFile() string {
	return filepath.Clean(filepath.Join(p.workspacePath, "ssh", "key"))
}

func (p *PathManager) GetLogsDir() string {
	return filepath.Join(p.workspacePath, "logs")
}

func (p *PathManager) GetRootfsDir() string {
	return filepath.Join(p.workspacePath, "rootfs")
}

func (p *PathManager) GetContainerStorageDiskPath() string {
	return filepath.Join(p.workspacePath, "raw-disk", "container-storage.ext4")
}
