package main

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
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

var errProcessExitNormal = errors.New("process exit normally")

func main() {
	if err := bootstrap(); err != nil && !errors.Is(err, errProcessExitNormal) {
		logrus.Fatal(err)
	}
}

func bootstrap() error {
	if err := filesystem.MountTmpfs(); err != nil {
		return err
	}

	if err := filesystem.LoadVMConfigAndMountVirtioFS(filepath.Join("/", "vmconfig.json")); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(context.Background())

	g.Go(func() error {
		return configureNetwork()
	})

	g.Go(func() error {
		return ssh.StartSSHServer(ctx)
	})

	g.Go(func() error {
		return doExecCmdLine(ctx, os.Args[1], os.Args[2:])
	})

	return g.Wait()
}

func doExecCmdLine(ctx context.Context, targetBin string, targetBinArgs []string) error {
	cmd := exec.CommandContext(ctx, targetBin, targetBinArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	logrus.Infof("run cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cmdline %q exit with err: %w", cmd.Args, err)
	}

	return errProcessExitNormal
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
