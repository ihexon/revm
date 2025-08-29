//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/sirupsen/logrus"
)

// VMConfig Static virtual machine configuration.

type (
	Cmdline  define.Cmdline
	VMConfig define.VMConfig
)

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}

func (c *Cmdline) TryGetSystemProxyAndSetToCmdline() error {
	proxyInfo, err := network.GetAndNormalizeSystemProxy()
	if err != nil {
		return fmt.Errorf("failed to get and normalize system proxy: %w", err)
	}

	if proxyInfo.HTTP == nil {
		logrus.Warnf("no system http proxy found")
	} else {
		httpProxy := fmt.Sprintf("http_proxy=http://%s:%d", proxyInfo.HTTP.Host, proxyInfo.HTTP.Port)
		logrus.Infof("using system http proxy: %q", httpProxy)
		c.Env = append(c.Env, httpProxy)
	}

	if proxyInfo.HTTPS == nil {
		logrus.Warnf("no system https proxy found")
	} else {
		httpsProxy := fmt.Sprintf("https_proxy=http://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
		logrus.Infof("using system https proxy: %q", httpsProxy)
		c.Env = append(c.Env, httpsProxy)
	}

	return nil
}

func (vmc *VMConfig) GenerateSSHInfo() error {
	keyPair, err := ssh.GenerateHostSSHKeyPair(vmc.HostSSHKeyPair)
	if err != nil {
		return fmt.Errorf("failed to generate host ssh keypair for host: %w", err)
	}

	vmc.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	vmc.HostSSHPublicKey = string(keyPair.AuthorizedKey())

	// Fill the SSHInfo
	portInHostSide, err := network.GetAvailablePort()
	if err != nil {
		return fmt.Errorf("failed to get avaliable port: %w", err)
	}

	vmc.SSHInfo.GuestPort = define.DefaultGuestSSHPort
	vmc.SSHInfo.GuestAddr = define.DefaultGuestSSHAddr

	vmc.SSHInfo.HostPort = portInHostSide
	vmc.SSHInfo.HostAddr = define.DefaultSSHInHost
	vmc.SSHInfo.User = define.DefaultGuestUser
	vmc.SSHInfo.AuthorizationKeyFile = vmc.HostSSHKeyPair

	return nil
}

func (vmc *VMConfig) Lock() (*flock.Flock, error) {
	f := filepath.Join(vmc.RootFS, define.LockFile)
	fileLock := flock.New(f)
	logrus.Infof("try to lock file: %s", f)
	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to lock file: %w", err)
	}

	if !ifLocked {
		return nil, fmt.Errorf("try lock file unsuccessful, mybe there is another vm instance running")
	}

	return fileLock, nil
}
