package services

import (
	"context"
	"linuxvm/pkg/network"
	"os"

	"github.com/sirupsen/logrus"
)

const (
	eth0     = "eth0"
	attempts = 3
)

func ConfigureNetwork(ctx context.Context) error {
	logrus.Infof("configure guest network: start")
	errChan := make(chan error, 1)

	go func() {
		verbose := false
		if _, find := os.LookupEnv("REVM_DEBUG"); find {
			verbose = true
		}

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			verbose = true
		}

		errChan <- network.DHClient4(eth0, attempts, verbose)
		logrus.Infof("configure guest network: done")
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}
