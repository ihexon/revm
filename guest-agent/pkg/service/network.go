package service

import (
	"context"
	"guestAgent/pkg/network"

	"github.com/sirupsen/logrus"
)

const (
	eth0     = "eth0"
	attempts = 3
)

func ConfigureNetwork(ctx context.Context) error {
	logrus.Infof("configure guest network: start")

	if err := network.DHClient4(ctx, eth0, attempts); err != nil {
		return err
	}
	logrus.Infof("configure guest network configure done")
	return nil
}
