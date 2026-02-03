package main

import (
	"context"
	"errors"
	"fmt"
	"guestAgent/pkg/service"
	"linuxvm/pkg/define"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

func setupLogger() error {
	level, err := logrus.ParseLevel(os.Getenv(define.EnvLogLevel))
	if err != nil {
		return err
	}
	logrus.SetLevel(level)

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})
	logrus.SetOutput(os.Stderr)
	return nil
}

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "rootfs guest agent",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "setup the guest environment, and run the command specified by the user.",
		Action:                    run,
		DisableSliceFlagSeparator: true,
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	if err := app.Run(ctx, os.Args); err != nil && !errors.Is(err, service.ErrProcessExitNormal) {
		logrus.Fatalf("guest-agent exit with error: %v", err)
	}
}

func mountAllFs(ctx context.Context, vmc *define.VMConfig) error {
	if err := service.MountAllPseudoMnt(ctx); err != nil {
		return err
	}

	if err := service.MountBlockDevices(ctx, vmc); err != nil {
		return err
	}

	return service.MountVirtiofs(ctx, vmc)
}

func run(ctx context.Context, _ *cli.Command) error {
	if err := setupLogger(); err != nil {
		return err
	}

	vmc, err := service.GetVMConfig(ctx)
	if err != nil {
		return err
	}

	if err := service.InitializeBusybox(); err != nil {
		return err
	}

	if err := mountAllFs(ctx, vmc); err != nil {
		return err
	}

	switch vmc.RunMode {
	case define.RootFsMode.String():
		return userRootfsMode(ctx, vmc)
	case define.ContainerMode.String():
		return dockerEngineMode(ctx, vmc)
	default:
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
}

func userRootfsMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("running in rootfs mode")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return service.ConfigureNetwork(ctx)
	})

	g.Go(func() error {
		return service.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return service.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		return service.DoExecCmdLine(ctx, vmc)
	})

	return g.Wait()
}

func dockerEngineMode(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Info("running in container mode")

	if !service.IsMounted(define.ContainerStorageMountPoint) {
		return fmt.Errorf("container storage is not mounted")
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return service.ConfigureNetwork(ctx)
	})

	g.Go(func() error {
		return service.StartPodmanAPIServices(ctx)
	})

	g.Go(func() error {
		return service.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return service.SyncRTCTime(ctx)
	})

	return g.Wait()
}
