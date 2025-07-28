package vmconfig

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"

	"os"
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
}

// Cmdline exec cmdline within rootfs
type Cmdline struct {
	Workspace     string   `json:"workspace,omitempty"`
	TargetBin     string   `json:"targetBin,omitempty"`
	TargetBinArgs []string `json:"targetBinArgs,omitempty"`
	Env           []string `json:"env,omitempty"`
}

func (c *Cmdline) UsingSystemProxy() error {
	proxyInfo, err := network.GetSystemProxy()
	if err != nil {
		return fmt.Errorf("failed to get system proxy: %v", err)
	}

	if proxyInfo.HTTP != nil && (proxyInfo.HTTP.Host == "127.0.0.1" || proxyInfo.HTTP.Host == "localhost") {
		logrus.Warnf("system http proxy is localhost, using gvproxy host ip instead")
		proxyInfo.HTTP.Host = "host.containers.internal"
	}

	if proxyInfo.HTTPS != nil && (proxyInfo.HTTPS.Host == "127.0.0.1" || proxyInfo.HTTPS.Host == "localhost") {
		logrus.Warnf("system https proxy is localhost, using gvproxy host ip instead")
		proxyInfo.HTTPS.Host = "host.containers.internal"
	}

	c.SetProxy(proxyInfo)

	return nil
}

func (c *Cmdline) SetProxy(proxyInfo *network.Proxy) {
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
		httpsProxy := fmt.Sprintf("https_proxy=https://%s:%d", proxyInfo.HTTPS.Host, proxyInfo.HTTPS.Port)
		logrus.Infof("using system https proxy: %q", httpsProxy)
		c.Env = append(c.Env, httpsProxy)
	}
}

func (vmc *VMConfig) WriteToJsonFile(file string) error {
	b, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}

	return os.WriteFile(file, b, 0644)
}
