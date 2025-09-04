package main

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/system"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

const (
	eth0     = "eth0"
	attempts = 1
)

var errProcessExitNormal = errors.New("process exit normally")

func setLogrus() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logrus.SetOutput(os.Stderr)
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "rootfs guest agent",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "setup the guest environment, and run the command specified by the user.",
		Before:                    earlyStage,
		Action:                    Bootstrap,
		DisableSliceFlagSeparator: true,
	}
	setLogrus()

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	if err := app.Run(ctx, os.Args); err != nil && !errors.Is(err, errProcessExitNormal) {
		logrus.Fatal(err)
	}
}

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	if err := filesystem.MountTmpfs(); err != nil {
		return ctx, err
	}

	if err := filesystem.LoadVMConfigAndMountVirtioFS(filepath.Join("/", define.VMConfigFile)); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func Bootstrap(ctx context.Context, command *cli.Command) error {
	vmc, err := define.LoadVMCFromFile(filepath.Join("/", define.VMConfigFile))
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	switch vmc.Cmdline.Mode {
	case define.RunUserCommandLineMode:
		return userCMDMode(ctx, vmc)
	case define.RunDockerEngineMode:
		return dockerEngineMode(ctx, vmc)
	default:
		return fmt.Errorf("unsupported mode %q", vmc.Cmdline.Mode)
	}
}

func dockerEngineMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Infof("run docker engine mode")
	return nil
}

func userCMDMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Infof("run user command line mode")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return configureNetwork()
	})

	g.Go(func() error {
		return ssh.StartSSHServer(ctx)
	})

	g.Go(func() error {
		return system.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		return doExecCmdLine(ctx, vmc.Cmdline.TargetBin, vmc.Cmdline.TargetBinArgs)
	})

	return g.Wait()
}

func doExecCmdLine(ctx context.Context, targetBin string, targetBinArgs []string) error {
	cmd := exec.CommandContext(ctx, targetBin, targetBinArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	logrus.Infof("full cmdline: %q", cmd.Args)
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
