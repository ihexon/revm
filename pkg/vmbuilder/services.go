//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	sshv2 "linuxvm/pkg/ssh_v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	sysproxy "github.com/ihexon/getSysProxy"
	"github.com/sirupsen/logrus"
)

func (v *VM) configureGuestAgent(ctx context.Context, pathMgr *PathManager) error {
	if v.WorkspacePath == "" {
		return fmt.Errorf("workspace path is empty")
	}

	unixUSL := &url.URL{
		Scheme: "unix",
		Path:   pathMgr.GetIgnAddr(),
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

	guestAgentFilePath := filepath.Join(v.RootFS, ".bin", "guest-agent")

	if err := os.MkdirAll(filepath.Dir(guestAgentFilePath), 0755); err != nil {
		return err
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	helperGuestAgent := filepath.Join(filepath.Dir(execPath), "..", "helper", "guest-agent")
	guestAgentBytes, err := os.ReadFile(helperGuestAgent)
	if err != nil {
		return fmt.Errorf("failed to read guest-agent from %q: %w", helperGuestAgent, err)
	}

	if err := os.WriteFile(guestAgentFilePath, guestAgentBytes, 0755); err != nil {
		return fmt.Errorf("failed to write guest-agent file to %q: %w", guestAgentFilePath, err)
	}

	v.GuestAgentCfg = define.GuestAgentCfg{
		Workdir: "/",
		Env:     finalEnv,
	}

	return nil
}

func (v *VM) configurePodman(ctx context.Context, pathMgr *PathManager) error {
	var envs []string

	if v.ProxySetting.Use {
		envs = append(envs, "http_proxy="+v.ProxySetting.HTTPProxy)
		envs = append(envs, "https_proxy="+v.ProxySetting.HTTPSProxy)
	}

	podmanProxyAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetPodmanListenAddr(),
	}

	port, err := network.GetAvailablePort(0)
	if err != nil {
		return err
	}

	listenIP := define.UnspecifiedAddress
	if v.VirtualNetworkMode == define.TSI {
		listenIP = define.LocalHost
	}

	v.PodmanInfo = define.PodmanInfo{
		HostPodmanProxyAddr:      podmanProxyAddr.String(),
		GuestPodmanAPIListenAddr: net.JoinHostPort(listenIP, strconv.FormatUint(port, 10)),
		GuestPodmanRunWithEnvs:   envs,
	}

	if err := os.MkdirAll(filepath.Dir(podmanProxyAddr.Path), 0755); err != nil {
		return err
	}

	return nil
}

func (v *VM) configureSSH(pathMgr *PathManager) error {
	keyPath := pathMgr.GetSSHPrivateKeyFile()
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

	v.SSHInfo = define.SSHInfo{
		HostSSHPublicKey:       string(publicKey),
		HostSSHPrivateKey:      string(privateKey),
		HostSSHPrivateKeyFile:  keyPath,
		GuestSSHPrivateKeyPath: "/run/dropbear/private.key",
		GuestSSHAuthorizedKeys: "/run/dropbear/authorized_keys",
		GuestSSHPidFile:        "/run/dropbear/dropbear.pid",
	}

	return nil
}

func (v *VM) configureVMCtlAPI(pathMgr *PathManager) error {
	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   pathMgr.GetVMCtlAddr(),
	}

	v.VMCtlAddress = unixAddr.String()

	if err := os.MkdirAll(filepath.Dir(unixAddr.Path), 0755); err != nil {
		return err
	}
	if err := os.Remove(unixAddr.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (v *VM) applySystemProxy() error {
	httpProxy, err := sysproxy.GetHTTP()
	if err != nil {
		return fmt.Errorf("get system proxy fail: %w", err)
	}

	if httpProxy == nil {
		logrus.Warnf("system proxy is not enabled, do nothing")
		return nil
	}

	if v.VirtualNetworkMode == define.GVISOR && (strings.Contains(httpProxy.String(), "127.0.0.1") ||
		strings.Contains(httpProxy.String(), "localhost")) {
		logrus.Debugf("in gvisor network mode, reset proxy to %s", define.HostDomainInGVPNet)
		httpProxy.Host = define.HostDomainInGVPNet
	}

	logrus.Infof("set http/https proxy to %s", httpProxy.String())
	v.ProxySetting = define.ProxySetting{
		Use:        true,
		HTTPProxy:  httpProxy.String(),
		HTTPSProxy: httpProxy.String(),
	}
	return nil
}
