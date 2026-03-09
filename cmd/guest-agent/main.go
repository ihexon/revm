package main

import (
	"context"
	"errors"
	"fmt"
	"guestAgent/pkg/machine"
	"guestAgent/pkg/service"
	"io"
	"linuxvm/pkg/define"
	commonlog "linuxvm/pkg/log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

// exitCode wraps a process exit code as an error so it can flow back
// through the error return chain to main(), which is the single os.Exit point.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

func setupLogger() error {
	level := strings.ToLower(os.Getenv(define.EnvLogLevel))
	if level == "" {
		level = "info"
	}
	_, err := commonlog.SetupLogger(level, "guest-agent", "")
	return err
}

// attachGuestLogPort finds the "guest-logs" virtio-console port and adds it
// as an additional logrus output. Must be called after /sys is mounted.
func attachGuestLogPort() {
	// f no need to be close
	f, err := openVirtioPortByName(define.GuestLogConsolePort)
	if err != nil {
		logrus.Debugf("guest-log port not available: %v", err)
		return
	}
	stderrWriter := io.MultiWriter(os.Stderr, f)
	logrus.SetOutput(stderrWriter)
	service.SetStderrWriter(stderrWriter)
	logrus.Infof("guest logs attached to virtio port %s", f.Name())
}

// openVirtioPortByName scans /sys/class/virtio-ports/*/name to find
// the device node for the given port name, then opens it for writing.
func openVirtioPortByName(name string) (*os.File, error) {
	matches, err := filepath.Glob("/sys/class/virtio-ports/*/name")
	if err != nil {
		return nil, err
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			// path: /sys/class/virtio-ports/vport1p0/name → device: /dev/vport1p0
			devName := filepath.Base(filepath.Dir(path))
			return os.OpenFile("/dev/"+devName, os.O_WRONLY, 0)
		}
	}
	return nil, fmt.Errorf("virtio port %q not found", name)
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

	if err := app.Run(ctx, os.Args); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		logrus.Error(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, _ *cli.Command) error {
	if err := setupLogger(); err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}

	if err := service.InitBinDir(); err != nil {
		return fmt.Errorf("init bin dir: %w", err)
	}

	vmc, err := service.GetVMConfig(ctx)
	if err != nil {
		return fmt.Errorf("get vm config: %w", err)
	}

	if err := service.MountAllPseudoMnt(ctx); err != nil {
		return fmt.Errorf("mount pseudo filesystems: %w", err)
	}

	// Now that /sys is available, attach the guest-logs virtio port
	attachGuestLogPort()

	// 4. Mount block devices and virtiofs
	if err := service.MountBlockDevices(ctx, vmc); err != nil {
		return fmt.Errorf("mount block devices: %w", err)
	}
	if err := service.MountVirtiofs(ctx, vmc); err != nil {
		return fmt.Errorf("mount virtiofs: %w", err)
	}

	// 5. Run mode-specific services
	switch vmc.RunMode {
	case define.RootFsMode.String():
		return userRootfsMode(ctx, vmc)
	case define.ContainerMode.String():
		return dockerEngineMode(ctx, vmc)
	default:
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
}

func userRootfsMode(ctx context.Context, vmc *define.Machine) error {
	logrus.Info("running in rootfs mode")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return service.ConfigureNetwork(ctx, (*machine.Machine)(vmc).GetVirtualNetworkType())
	})

	g.Go(func() error {
		return service.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		return service.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		return exitCode(service.DoExecCmdLine(ctx, vmc))
	})

	g.Go(func() error {
		return machine.WaitGuestServiceReady(ctx, vmc)
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- g.Wait()
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}

func dockerEngineMode(ctx context.Context, vmc *define.Machine) error {
	logrus.Info("starting container engine")

	if err := service.SetupContainerStorage(vmc); err != nil {
		return fmt.Errorf("setup container storage: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return service.ConfigureNetwork(ctx, (*machine.Machine)(vmc).GetVirtualNetworkType())
	})

	g.Go(func() error {
		return service.StartPodmanAPIServices(ctx, vmc)
	})

	g.Go(func() error {
		return service.StartGuestSSHServer(ctx, vmc)
	})

	// time sync error does not matter
	g.Go(func() error {
		return service.SyncRTCTime(ctx)
	})

	g.Go(func() error {
		return machine.WaitGuestServiceReady(ctx, vmc)
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- g.Wait()
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}
