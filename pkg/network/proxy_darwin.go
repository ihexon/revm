//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"fmt"
	"github.com/oomol-lab/sysproxy"
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
