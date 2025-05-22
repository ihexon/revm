//go:build linux

package main

import (
	"context"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"linuxvm/pkg/network"
	"os"
	"os/exec"
)

const (
	eth0     = "eth0"
	attempts = 1
	verbose  = false
)

func main() {
	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		err := network.DHClient4(eth0, attempts, verbose)
		if err != nil {
			logrus.Errorf("failed to get dhcp config: %v", err)
			return err
		}
		return nil
	})

	g.Go(func() error {
		cmd := exec.CommandContext(ctx, os.Args[1], os.Args[2:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		logrus.Infof("%q", cmd.Args)
		return cmd.Run()
	})

	if err := g.Wait(); err != nil {
		logrus.Errorf("failed to run cmd: %v", err)
	}
}
