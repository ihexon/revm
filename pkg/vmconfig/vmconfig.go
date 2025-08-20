//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmconfig

import (
	"encoding/json"
	"fmt"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"os"

	"github.com/sirupsen/logrus"
)

// VMConfig Static virtual machine configuration.
type VMConfig struct {
	MemoryInMB int32  `json:"memoryInMB,omitempty"`
	Cpus       int8   `json:"cpus,omitempty"`
	RootFS     string `json:"rootFS,omitempty"`

	// data disk will map into /dev/vdX
	DataDisk []string `json:"dataDisk,omitempty"`
	// GVproxy control endpoint
	GVproxyEndpoint string `json:"GVproxyEndpoint,omitempty"`
	// NetworkStackBackend is the network stack backend to use. which provided
	// by gvproxy
	NetworkStackBackend string             `json:"networkStackBackend,omitempty"`
	LogLevel            string             `json:"logLevel,omitempty"`
	Mounts              []filesystem.Mount `json:"mounts,omitempty"`
	PortForwardMap      map[string]string  `json:"portForwardMap,omitempty"`

	HostSSHKeyPair    string `json:"hostSSHKeyPair,omitempty"`
	HostSSHPublicKey  string `json:"sshPublicKey,omitempty"`
	HostSSHPrivateKey string `json:"sshPrivateKey,omitempty"`
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	Workspace     string   `json:"workspace,omitempty"`
	TargetBin     string   `json:"targetBin,omitempty"`
	TargetBinArgs []string `json:"targetBinArgs,omitempty"`
	Env           []string `json:"env,omitempty"`
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

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}

func (vmc *VMConfig) GenerateSSHKeyPairForHost() error {
	keyPair, err := ssh.GenerateHostSSHKeyPair(vmc.HostSSHKeyPair)
	if err != nil {
		return fmt.Errorf("failed to generate host ssh keypair for host: %w", err)
	}

	logrus.Debugf("host ssh keypair private: %q", keyPair.RawProtectedPrivateKey())
	vmc.HostSSHPrivateKey = string(keyPair.RawProtectedPrivateKey())
	logrus.Debugf("host ssh keypair public: %q", keyPair.AuthorizedKey())
	vmc.HostSSHPublicKey = string(keyPair.AuthorizedKey())

	return nil
}
