package main

import (
	"context"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
)

const (
	eth0     = "eth0"
	attempts = 1
	verbose  = true
)

func main() {
	g, _ := errgroup.WithContext(context.Background())
	g.Go(func() error {
		err := network.DHClient4(eth0, attempts, verbose)
		if err != nil {
			logrus.Errorf("failed to get dhcp config: %v", err)
			return err
		}
		return nil
	})

	g.Go(func() error {
		return filesystem.MountTmpfs()
	})

	if err := g.Wait(); err != nil {
		logrus.Errorf("failed to run cmd: %v", err)
	}
}
