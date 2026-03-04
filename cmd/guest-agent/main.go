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

func setupLogger() error {
	level := strings.ToLower(os.Getenv(define.EnvLogLevel))
	if level == "" {
		level = "info"
	}
	return commonlog.SetupBasicLoggerWithStage(level, "guest-agent")
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

	if err := app.Run(ctx, os.Args); err != nil && !errors.Is(err, service.ErrProcessExitNormal) {
		if cause := context.Cause(ctx); cause != nil && cause != err {
			logrus.Fatalf("guest-agent exit with error: %v (cause: %v)", err, cause)
		}
		logrus.Fatalf("guest-agent exit with error: %v", err)
	}
}

func run(ctx context.Context, _ *cli.Command) error {
	if err := setupLogger(); err != nil {
		return err
	}

	if err := service.InitBinDir(); err != nil {
		return fmt.Errorf("init bin dir: %w", err)
	}

	// 2. Get VM configuration from host
	vmc, err := service.GetVMConfig(ctx)
	if err != nil {
		return err
	}

	// 3. Mount all pseudo filesystems (/proc, /sys, /dev, /tmp, etc.)
	if err := service.MountAllPseudoMnt(ctx); err != nil {
		return err
	}

	// Now that /sys is available, attach the guest-logs virtio port
	attachGuestLogPort()

	// 4. Mount block devices and virtiofs
	if err := service.MountBlockDevices(ctx, vmc); err != nil {
		return err
	}
	if err := service.MountVirtiofs(ctx, vmc); err != nil {
		return err
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
		return service.DoExecCmdLine(ctx, vmc)
	})

	g.Go(func() error {
		return machine.WaitGuestServiceReady(ctx, vmc)
	})

	return g.Wait()
}

func dockerEngineMode(ctx context.Context, vmc *define.Machine) error {
	logrus.Info("running in container mode")

	if !service.IsMounted(define.ContainerStorageMountPoint) {
		return fmt.Errorf("container storage is not mounted")
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

	return g.Wait()
}
