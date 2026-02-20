//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	sshv2 "linuxvm/pkg/ssh_v2"
	"linuxvm/pkg/static_resources"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// --- Guest Agent ---

// GuestAgentConfigurator handles guest agent configuration.
type GuestAgentConfigurator struct {
	pathMgr *PathManager
}

// NewGuestAgentConfigurator creates a new guest agent configurator.
func NewGuestAgentConfigurator(pathMgr *PathManager) *GuestAgentConfigurator {
	return &GuestAgentConfigurator{pathMgr: pathMgr}
}

// Configure sets up the guest agent including ignition server and environment variables.
func (g *GuestAgentConfigurator) Configure(ctx context.Context, vmc *define.Machine) error {
	if vmc.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   g.pathMgr.GetIgnAddr(),
	}

	if err := os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	if err := os.Remove(unixUSL.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	vmc.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenSockAddr: unixUSL.String(),
	}

	if vmc.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	// inject user-given envs to guest-agent
	var finalEnv []string
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "TMPDIR=/tmp")

	// In the virtualNetwork, HOST_DOMAIN is the domain name of the host, and the guest can access network resources on the host through HOST_DOMAIN.
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomainInGVPNet))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	guestAgentFilePath := filepath.Join(vmc.RootFS, ".bin", "guest-agent")

	if err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(guestAgentFilePath, static_resources.GuestAgentBytes, 0755); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	vmc.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

// --- Podman ---

// PodmanConfigurator handles Podman service configuration.
type PodmanConfigurator struct {
	pathMgr *PathManager
}

// NewPodmanConfigurator creates a new Podman configurator.
func NewPodmanConfigurator(pathMgr *PathManager) *PodmanConfigurator {
	return &PodmanConfigurator{pathMgr: pathMgr}
}

// Configure sets up Podman API configuration including proxy settings.
func (p *PodmanConfigurator) Configure(ctx context.Context, vmc *define.Machine) error {
	var envs []string

	if vmc.ProxySetting.Use {
		envs = append(envs, "http_proxy="+vmc.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+vmc.ProxySetting.HTTPSProxy)
	}

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   p.pathMgr.GetPodmanListenAddr(),
	}

	vmc.PodmanInfo = define.PodmanInfo{
		PodmanProxyAddr:    podmanProxyAddr.String(),
		GuestPodmanAPIIP:   define.GuestIP,
		GuestPodmanAPIPort: define.GuestPodmanAPIPort,
		Envs:               envs,
	}

	if err := os.MkdirAll(filepath.Dir(podmanProxyAddr.Path), 0755); err != nil {
		return err
	}

	return nil
}

// --- SSH ---

// SSHConfigurator handles SSH key generation and configuration.
type SSHConfigurator struct {
	pathMgr *PathManager
}

// NewSSHConfigurator creates a new SSH configurator.
func NewSSHConfigurator(pathMgr *PathManager) *SSHConfigurator {
	return &SSHConfigurator{pathMgr: pathMgr}
}

// Configure generates SSH keys and sets up SSH configuration.
func (s *SSHConfigurator) Configure(ctx context.Context, vmc *define.Machine) error {
	keyPath := s.pathMgr.GetSSHPrivateKeyFile()
	pubKeyPath := keyPath + ".pub"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return err
	}

	privateKey, publicKey, err := sshv2.GenerateKey()
	if err != nil {
		return err
	}
	if err = os.WriteFile(keyPath, privateKey, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(pubKeyPath, publicKey, 0644); err != nil {
		return err
	}

	vmc.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      string(publicKey),
		HostSSHPrivateKey:     string(privateKey),
		HostSSHPrivateKeyFile: keyPath,
	}

	return nil
}
