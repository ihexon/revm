//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	ssh "linuxvm/pkg/ssh"
	"linuxvm/pkg/static_resources"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	sysproxy "github.com/ihexon/getSysProxy"
	"github.com/sirupsen/logrus"
)

type machineBuilder struct {
	define.MachineSpec
	pathMgr *machinePathManager
}

func newMachineBuilder(mode define.RunMode) *machineBuilder {
	return &machineBuilder{
		MachineSpec: define.MachineSpec{
			RunMode: mode.String(),
		},
	}
}

func (v *machineBuilder) setupWorkspace(workspacePath string) error {
	if workspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	workspacePath, err := filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return err
	}

	v.WorkspaceDir = workspacePath

	if err = os.MkdirAll(v.WorkspaceDir, 0700); err != nil {
		return err
	}

	if err := os.Chmod(v.WorkspaceDir, 0700); err != nil {
		return err
	}
	// libkrun creates its Unix listeners after the VM starts. A private
	// parent protects them from creation onward, including custom symlinks.

	v.pathMgr = newMachinePathManager(v.WorkspaceDir)

	return nil
}

func (v *machineBuilder) withBuiltInAlpineRootfs(ctx context.Context) error {
	if v.WorkspaceDir == "" {
		return fmt.Errorf("workspace path is empty")
	}

	alpineRootfsDir := v.pathMgr.GetRootfsDir()
	if err := os.MkdirAll(alpineRootfsDir, 0755); err != nil {
		return err
	}

	if err := static_resources.ExtractBuiltinRootfs(ctx, alpineRootfsDir); err != nil {
		return err
	}
	v.RootFS = alpineRootfsDir
	return nil
}

func (v *machineBuilder) withMountUserHome(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return v.withUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})
}

func (v *machineBuilder) withUserProvidedMounts(dirs []string) error {
	if len(dirs) == 0 || dirs == nil {
		return nil
	}

	mounts := filesystem.CmdLineMountToMounts(dirs)
	for i := range mounts {
		p, err := filepath.Abs(filepath.Clean(mounts[i].Source))
		if err != nil {
			return fmt.Errorf("failed to resolve mount source %q: %w", mounts[i].Source, err)
		}
		mounts[i].Source = p
	}
	v.Storage.VirtioFS = append(v.Storage.VirtioFS, mounts...)
	return nil
}

func (v *machineBuilder) configureGuestAgent(ctx context.Context) error {
	if v.WorkspaceDir == "" {
		return fmt.Errorf("workspace path is empty")
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   v.pathMgr.GetIgnSocketFile(),
	}

	if err := os.MkdirAll(filepath.Dir(unixUSL.Path), 0755); err != nil {
		return err
	}

	if err := os.Remove(unixUSL.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	v.IgnitionServerCfg = define.IgnitionServerCfg{
		ListenSockAddr: unixUSL.String(),
	}
	controlURL := &url.URL{Scheme: "unix", Path: v.pathMgr.GetGuestControlSocketFile()}
	v.GuestControlAddr = controlURL.String()
	if err := os.Remove(controlURL.Path); err != nil && !os.IsNotExist(err) {
		return err
	}

	if v.RootFS == "" {
		return fmt.Errorf("rootfs path is empty")
	}

	var finalEnv []string
	finalEnv = append(finalEnv, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	finalEnv = append(finalEnv, "LC_ALL=C.UTF-8")
	finalEnv = append(finalEnv, "LANG=C.UTF-8")
	finalEnv = append(finalEnv, "TMPDIR=/tmp")
	finalEnv = append(finalEnv, fmt.Sprintf("HOST_DOMAIN=%s", define.HostDomainInGVPNet))
	finalEnv = append(finalEnv, fmt.Sprintf("%s=%s", define.EnvLogLevel, logrus.GetLevel().String()))

	if _, err := static_resources.GuestAgent(); err != nil {
		return err
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

func (v *machineBuilder) configurePodman(ctx context.Context, userEnv []string) error {
	envs := append([]string{}, userEnv...)

	apiPath := v.pathMgr.GetPodmanSocketFile()

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   apiPath,
	}

	v.PodmanInfo = define.PodmanInfo{
		HostPodmanProxyAddr:     podmanProxyAddr.String(),
		HostPodmanTransportAddr: (&url.URL{Scheme: "unix", Path: v.pathMgr.GetPodmanTransportSocketFile()}).String(),
		GuestPodmanRunWithEnvs:  envs,
	}

	if err := os.MkdirAll(filepath.Dir(podmanProxyAddr.Path), 0755); err != nil {
		return err
	}
	if err := os.Remove(podmanProxyAddr.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(v.pathMgr.GetPodmanTransportSocketFile()); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (v *machineBuilder) configureSSH() error {
	keyPath := v.pathMgr.GetSSHKeyFilePath()
	pubKeyPath := keyPath + ".pub"
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return err
	}

	privateKey, publicKey, err := ssh.GenerateKey()
	if err != nil {
		return err
	}
	if err = os.WriteFile(keyPath, privateKey, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(pubKeyPath, publicKey, 0644); err != nil {
		return err
	}

	v.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:      string(publicKey),
		HostSSHPrivateKey:     string(privateKey),
		HostSSHPrivateKeyFile: keyPath,

		GuestSSHPrivateKeyFile: "/run/dropbear/private.key",
		GuestSSHAuthorizedKeys: "/root/.ssh/authorized_keys",
		GuestSSHPidFile:        "/run/dropbear/dropbear.pid",
	}

	return nil
}

func (v *machineBuilder) configureVMCtlAPI() error {
	apiPath := v.pathMgr.GetVMCtlSocketFile()

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   apiPath,
	}

	v.VMCtlAddr = unixAddr.String()

	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}
	if err := os.Remove(unixAddr.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// createSymlink never replaces an unrelated file or connection endpoint.
func createSymlink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	if _, err := os.Lstat(linkPath); err == nil {
		if existing, e := os.Readlink(linkPath); e == nil && existing == target {
			return nil
		}
		return fmt.Errorf("path already exists: %s; remove the stale link explicitly", linkPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, linkPath)
}

var getSystemHTTPProxy = sysproxy.GetHTTP

func systemProxyEnv(network define.VNetMode) ([]string, error) {
	httpProxy, err := getSystemHTTPProxy()
	if err != nil {
		return nil, fmt.Errorf("get system proxy fail: %w", err)
	}

	if httpProxy == nil {
		return nil, nil
	}

	proxyHost := strings.ToLower(strings.Trim(httpProxy.Host, "[]"))
	if network == define.GVISOR &&
		(proxyHost == "127.0.0.1" || proxyHost == "localhost" || proxyHost == "::1") {
		httpProxy.Host = define.HostDomainInGVPNet
	}

	return []string{"http_proxy=" + httpProxy.String(), "https_proxy=" + httpProxy.String()}, nil
}

func pathWithin(base, candidate string) bool {
	base, err := filepath.Abs(filepath.Clean(base))
	if err != nil {
		return false
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// buildMachine prepares native devices; the session lease owns every file.
func buildMachine(ctx context.Context, cfg Config, workspace string) (*define.MachineSpec, error) {
	mode := define.RootFsMode
	if cfg.RunMode == ModeContainer {
		mode = define.ContainerMode
	}
	b := newMachineBuilder(mode)
	if err := b.setupWorkspace(workspace); err != nil {
		return nil, err
	}
	b.pathMgr.rootfsDir = filepath.Join(cfg.paths.record.CacheDir, "rootfs")
	b.LogFile = filepath.Join(cfg.paths.logDir, "vm.log")
	b.MemoryInMB, b.Cpus = cfg.MemoryMB, uint8(cfg.CPUs)
	if err := b.configureSSH(); err != nil {
		return nil, err
	}
	if err := b.configureNetwork(define.VNetMode(cfg.Network)); err != nil {
		return nil, err
	}
	if err := validatePortForwardSet(cfg.PortForwards, b.SSHInfo.HostSSHProxyListenAddr); err != nil {
		return nil, err
	}
	b.PortForwards = cfg.PortForwards
	if err := b.withBuiltInAlpineRootfs(ctx); err != nil {
		return nil, err
	}
	if cfg.RunMode == ModeContainer {
		if err := b.withMountUserHome(ctx); err != nil {
			return nil, err
		}
		if err := b.configurePodman(ctx, cfg.Exec.Env); err != nil {
			return nil, err
		}
		if err := b.configureContainerRAWDisk(ctx, cfg.ContainerDisk, ""); err != nil {
			return nil, err
		}
	}
	if err := b.withUserProvidedStorageRAWDisk(ctx, cfg.Disks); err != nil {
		return nil, err
	}
	if err := b.withUserProvidedMounts(cfg.Mounts); err != nil {
		return nil, err
	}
	if err := b.configureGuestAgent(ctx); err != nil {
		return nil, err
	}
	if err := b.configureVMCtlAPI(); err != nil {
		return nil, err
	}
	return &b.MachineSpec, nil
}
