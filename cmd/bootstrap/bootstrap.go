package main

import (
	"context"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	eth0     = "eth0"
	attempts = 1
)

func main() {
	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		return configureNetwork()
	})

	g.Go(func() error {
		return filesystem.MountTmpfs()
	})

	g.Go(func() error {
		return filesystem.MountVirtioFS(filepath.Join("/", "vmconfig.json"))
	})

	g.Go(func() error {
		return doExecCmdLine(ctx, os.Args[1], os.Args[2:])
	})

	if err := g.Wait(); err != nil {
		logrus.Errorf("failed to run cmd: %v", err)
	}
}

func doExecCmdLine(ctx context.Context, targetBin string, targetBinArgs []string) error {
	cmd := exec.CommandContext(ctx, targetBin, targetBinArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	logrus.Infof("cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		logrus.Errorf("failed to run cmd: %v", err)
		return err
	}
	return nil
}

func configureNetwork() error {
	verbose := false
	if _, find := os.LookupEnv("REVM_DEBUG"); find {
		verbose = true
	}

	if err := network.DHClient4(eth0, attempts, verbose); err != nil {
		logrus.Errorf("failed to get dhcp config: %v", err)
		return err
	}
	return nil
}
