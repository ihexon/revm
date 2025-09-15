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

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

const (
	eth0     = "eth0"
	attempts = 3
)

var errProcessExitNormal = errors.New("process exit normally")

func setLogrus(command *cli.Command) {
	logrus.SetLevel(logrus.InfoLevel)
	if command.Bool(define.FlagVerbose) {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		ForceColors:            true,
	})
	logrus.SetOutput(os.Stderr)
}

var dhcpDoneChan = make(chan struct{}, 1)

func main() {
	app := cli.Command{
		Name:        os.Args[0],
		Usage:       "rootfs guest agent",
		UsageText:   os.Args[0] + " [command] [flags]",
		Description: "setup the guest environment, and run the command specified by the user.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:   define.FlagVerbose,
				Hidden: true,
				Value:  false,
			},
		},
		Before:                    earlyStage,
		Action:                    Bootstrap,
		DisableSliceFlagSeparator: true,
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	if err := app.Run(ctx, os.Args); err != nil && !errors.Is(err, errProcessExitNormal) {
		logrus.Fatalf("bootstrap exit with error: %v", err)
	}

	logrus.Debugf("bootstrap exit normally")
}

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	setLogrus(command)

	if err := filesystem.MountTmpfs(); err != nil {
		return ctx, err
	}

	if err := filesystem.LoadVMConfigAndMountVirtioFS(ctx); err != nil {
		return ctx, err
	}
	if err := filesystem.LoadVMConfigAndMountDataDisk(ctx); err != nil {
		return ctx, err
	}

	return ctx, nil
}

func Bootstrap(ctx context.Context, command *cli.Command) error {
	vmc, err := define.LoadVMCFromFile(filepath.Join("/", define.VMConfigFile))
	if err != nil {
		return fmt.Errorf("failed to load vmconfig: %w", err)
	}

	switch vmc.RunMode {
	case define.RunUserRootfsMode:
		return userRootfsMode(ctx, vmc)
	case define.RunDockerEngineMode:
		return dockerEngineMode(ctx, vmc)
	default:
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
}

func StartSSHServer(ctx context.Context, vmc *define.VMConfig) error {
	cfg := ssh.SSHServer{
		Port:     vmc.SSHInfo.Port,
		Provider: ssh.TypeDropbear,
		Addr:     "0.0.0.0",
	}

	return ssh.StartSSHServer(ctx, cfg)
}

func checkContainerStorageMounted() error {
	mounted, err := mountinfo.Mounted(define.ContainerStorageMountPoint)
	if err != nil {
		return fmt.Errorf("failed to check %q mounted: %w", define.ContainerStorageMountPoint, err)
	}
	if !mounted {
		return fmt.Errorf("container storage %q is not mounted", define.ContainerStorageMountPoint)
	}
	return nil
}

func dockerEngineMode(ctx context.Context, vmc *define.VMConfig) error {
	// docker mode need container storage mounted, so we check it first
	if err := checkContainerStorageMounted(); err != nil {
		return fmt.Errorf("failed to check container storage mounted: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return configureNetwork(ctx)
	})

	g.Go(func() error {
		return StartSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return system.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		logrus.Info("start podman API service in guest")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-dhcpDoneChan:
			logrus.Debugf("dhcp done, start podman service")
			return system.StartPodmanService(ctx)
		}
	})

	return g.Wait()
}

func userRootfsMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Debugf("run user command line mode")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return configureNetwork(ctx)
	})

	g.Go(func() error {
		return StartSSHServer(ctx, vmc)
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

	logrus.Debugf("full cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cmdline %q exit with err: %w", cmd.Args, err)
	}

	return errProcessExitNormal
}

func configureNetwork(ctx context.Context) error {
	logrus.Infof("configure guest network: start")
	errChan := make(chan error)

	go func() {
		verbose := false
		if _, find := os.LookupEnv("REVM_DEBUG"); find {
			verbose = true
		}
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			verbose = true
		}
		errChan <- network.DHClient4(eth0, attempts, verbose)
		// mark the dhcp operation finished
		dhcpDoneChan <- struct{}{}
		logrus.Debugf("configure guest network: dhcp done")
		close(dhcpDoneChan)
		logrus.Infof("configure guest network: done")
	}()

	defer close(errChan)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:

		return err
	}
}
