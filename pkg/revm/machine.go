//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/static_resources"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

type machineBuilder struct {
	define.MachineSpec
	pathMgr *machinePathManager
}

func newMachineBuilder(mode define.RunMode) *machineBuilder {
	return &machineBuilder{
		MachineSpec: define.MachineSpec{
			RunMode: mode,
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

	return os.MkdirAll(filepath.Dir(v.pathMgr.GetGuestControlSocketFile()), 0700)
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

func (v *machineBuilder) configureGuestAgent(ctx context.Context) error {
	if v.WorkspaceDir == "" {
		return fmt.Errorf("workspace path is empty")
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
	hostDomain := define.HostDomainInGVPNet
	if v.Network == define.NetworkTSI {
		hostDomain = "localhost"
	}
	finalEnv = append(finalEnv, "HOST_DOMAIN="+hostDomain)
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
func buildMachine(ctx context.Context, cfg VMOptions, lease *sessionRun) (*define.MachineSpec, error) {
	b := newMachineBuilder(cfg.RunMode)
	if err := b.setupWorkspace(lease.record.RuntimeDir); err != nil {
		return nil, err
	}
	b.pathMgr.rootfsDir = filepath.Join(lease.record.CacheDir, "rootfs")
	b.LogFile = filepath.Join(lease.logDir, "vm.log")
	b.MemoryInMB, b.Cpus = cfg.MemoryMB, uint8(cfg.CPUs)
	b.Network = cfg.Network
	if err := b.configureNetwork(); err != nil {
		return nil, err
	}
	b.PortForwards = cfg.PortForwards
	if err := b.withBuiltInAlpineRootfs(ctx); err != nil {
		return nil, err
	}
	if cfg.RunMode == define.ContainerMode {
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
	b.Storage.VirtioFS = cfg.Mounts
	if err := b.configureGuestAgent(ctx); err != nil {
		return nil, err
	}
	return &b.MachineSpec, nil
}
