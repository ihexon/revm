package main

import (
	"context"
	"errors"
	"fmt"
	"guestAgent/pkg/service"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/protocol"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func setupLogger() {
	level := strings.ToLower(os.Getenv(define.EnvLogLevel))
	if level == "" {
		level = "info"
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		l = logrus.InfoLevel
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	return
}

// setupGuestLogPort opens the guest-log port and routes guest logs to it.
func setupGuestLogPort() {
	f, err := openVirtioPort(define.GuestLogConsolePort, os.O_WRONLY)
	if err != nil {
		logrus.Debugf("guest-log port not available: %v", err)
		return
	}

	// Guest logs share the same host-side session file as host lifecycle logs.
	// Do not mirror them to the guest TTY: doing so races with the user's
	// command output and produces visually corrupted terminal lines.
	logrus.SetOutput(f)
	service.SetStderrWriter(f)
	logrus.Infof("guest logs attached to virtio port %s", f.Name())
}

// openVirtioPort scans /sys/class/virtio-ports/*/name to find
// the device node for the given port name, then opens it with the specified flags.
func openVirtioPort(name string, flag int) (*os.File, error) {
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
			devName := filepath.Base(filepath.Dir(path))
			return os.OpenFile("/dev/"+devName, flag, 0)
		}
	}
	return nil, fmt.Errorf("virtio port %q not found", name)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "_exec" {
		err := execution.ExecIsolated(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "revm exec:", err)
			os.Exit(execution.FromProcess(err).ExitCode())
		}
		return
	}

	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "rootfs guest agent",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "setup the guest environment, and run the command specified by the user.",
		Action:                    run,
		DisableSliceFlagSeparator: true,
		ExitErrHandler:            func(context.Context, *cli.Command, error) {},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logrus.Errorf("cmdline exit unexpected: %v", err)
			os.Exit(execution.FromProcess(exitErr).ExitCode())
		}
		if errors.Is(err, exec.ErrNotFound) {
			logrus.Errorf("cmdline exit unexpected: %v", err)
			os.Exit(127)
		}

		logrus.Fatalf("cmdline exit unexpected: %v", err)
	}
}

func run(ctx context.Context, _ *cli.Command) error {
	setupLogger()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	vmc, err := service.GetVMConfig()
	if err != nil {
		return fmt.Errorf("get vm config: %w", err)
	}

	if err := service.MountAllPseudoMnt(ctx); err != nil {
		return fmt.Errorf("mount pseudo filesystems: %w", err)
	}

	// Now that /sys is available, wire up the dedicated guest log port.
	setupGuestLogPort()

	// 4. Mount block devices and virtiofs
	if err := service.MountBlockDevices(ctx, vmc); err != nil {
		return fmt.Errorf("mount block devices: %w", err)
	}
	if err := service.MountVirtiofs(ctx, vmc); err != nil {
		return fmt.Errorf("mount virtiofs: %w", err)
	}

	if err := service.ConfigureNetwork(ctx, virtualNetworkType(vmc)); err != nil {
		return err
	}
	execution.RequireGuestIsolation()
	return service.RunGuest(ctx, vmc)
}

func virtualNetworkType(vmc *protocol.GuestSpec) define.VNetMode {
	if vmc.NetworkMode == string(define.TSI) {
		return define.TSI
	}
	return define.GVISOR
}
