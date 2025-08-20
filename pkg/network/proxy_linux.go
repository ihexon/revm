//go:build linux && (arm64 || amd64)

package network

import (
	"github.com/oomol-lab/sysproxy"
)

type Proxy struct {
	HTTP  *sysproxy.Info
	HTTPS *sysproxy.Info
}

func GetSystemProxy() (*Proxy, error) {
	return nil, nil
}
