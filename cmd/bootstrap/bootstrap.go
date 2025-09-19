package main

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/cmd/bootstrap/pkg/services"
	"linuxvm/pkg/define"
	"os"
	"os/signal"
	"syscall"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

func setLogrus(command *cli.Command) {
	logrus.SetLevel(logrus.InfoLevel)
	if command.Bool(define.FlagVerbose) {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})
	logrus.SetOutput(os.Stderr)
}

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

	if err := app.Run(ctx, os.Args); err != nil && !errors.Is(err, services.ErrProcessExitNormal) {
		logrus.Fatalf("bootstrap exit with error: %v", err)
	}

	logrus.Debugf("bootstrap exit normally")
}

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	setLogrus(command)

	err := services.DownloadLinuxUtils(ctx)
	if err != nil {
		return ctx, err
	}

	if err := services.MountPseudoFilesystem(ctx); err != nil {
		return ctx, err
	}
	logrus.Infof("start guest bootstrap")

	return ctx, nil
}

func Bootstrap(ctx context.Context, command *cli.Command) error {
	vmc, err := services.NewVSockService().GetVMConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get vmconfig from vsock: %w", err)
	}

	// Mount the data disk(virtio-blk)
	if err = services.MountDataDisk(ctx, vmc); err != nil {
		return fmt.Errorf("failed to mount data disk: %w", err)
	}

	// Mount the host dir(virtiofs)
	if err = services.MountHostDir(ctx, vmc); err != nil {
		return fmt.Errorf("failed to mount host dir: %w", err)
	}

	switch vmc.RunMode {
	case define.RootFsMode.String():
		return userRootfsMode(ctx, vmc)
	case define.DockerMode.String():
		return dockerEngineMode(ctx, vmc)
	default:
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
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
		return services.ConfigureNetwork(ctx)
	})

	g.Go(func() error {
		return services.StartPodmanAPIServices(ctx)
	})

	g.Go(func() error {
		return services.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return services.SyncRTCTime(ctx)
	})

	return g.Wait()
}

func userRootfsMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Debugf("run user command line mode")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return services.ConfigureNetwork(ctx)
	})

	g.Go(func() error {
		return services.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return services.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		return services.DoExecCmdLine(ctx, vmc.Cmdline.TargetBin, vmc.Cmdline.TargetBinArgs)
	})

	return g.Wait()
}
