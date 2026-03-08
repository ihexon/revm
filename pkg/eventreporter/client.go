package eventreporter

import (
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func newClient(endpoint string) *network.Client {
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid unix endpoint %q: %v", endpoint, err)
			return nil
		}
		return network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid tcp endpoint %q: %v", endpoint, err)
			return nil
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		return network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		logrus.Warnf("event sink: unsupported endpoint scheme %q", endpoint)
		return nil
	}
}
