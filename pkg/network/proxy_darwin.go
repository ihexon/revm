//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"fmt"

	"github.com/oomol-lab/sysproxy"
	"github.com/sirupsen/logrus"
)

type Proxy struct {
	HTTP  *sysproxy.Info
	HTTPS *sysproxy.Info
}

func GetSystemProxy() (*Proxy, error) {
	httpInfo, err := sysproxy.GetHTTP()
	if err != nil {
		return nil, fmt.Errorf("failed to get system http proxy: %w", err)
	}

	httpsInfo, err := sysproxy.GetHTTPS()
	if err != nil {
		return nil, fmt.Errorf("failed to get system https proxy: %w", err)
	}

	return &Proxy{
		HTTP:  httpInfo,
		HTTPS: httpsInfo,
	}, nil
}

func GetAndNormalizeSystemProxy() (*Proxy, error) {
	proxyInfo, err := GetSystemProxy()
	if err != nil {
		return nil, fmt.Errorf("failed to get system proxy: %v", err)
	}

	if proxyInfo.HTTP != nil && (proxyInfo.HTTP.Host == "127.0.0.1" || proxyInfo.HTTP.Host == "localhost") {
		logrus.Warnf("system http proxy is localhost, using gvproxy host ip instead")
		proxyInfo.HTTP.Host = "host.containers.internal"
	}

	if proxyInfo.HTTPS != nil && (proxyInfo.HTTPS.Host == "127.0.0.1" || proxyInfo.HTTPS.Host == "localhost") {
		logrus.Warnf("system https proxy is localhost, using gvproxy host ip instead")
		proxyInfo.HTTPS.Host = "host.containers.internal"
	}

	return proxyInfo, nil
}
