package service

import (
	"context"
	"guestAgent/pkg/network"
	"linuxvm/pkg/define"
	"os"

	"github.com/sirupsen/logrus"
)

const (
	eth0     = "eth0"
	attempts = 3
)

const (
	resolveFile       = "/etc/resolv.conf"
	defaultNameServer = "nameserver 1.1.1.1"
)

// ConfigureNetwork must support TSI/Gvisor network
func ConfigureNetwork(ctx context.Context, mode define.VNetMode) error {
	if mode == define.TSI {
		logrus.Infof("set the Guest's default DNS to 1.1.1.1")
		return os.WriteFile(resolveFile, []byte(defaultNameServer), 0644)
	}

	return network.DHClient4(ctx, eth0, attempts)
}
