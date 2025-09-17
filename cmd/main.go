//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run a linux shell in 1 second",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "run a linux shell in 1 second",
		Before:                    earlyStage,
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: define.FlagRestAPIListenAddr,
				Usage: "listen for REST API requests on the given address, support http or unix socket address," +
					" e.g. http://127.0.0.1:8080 or unix:///tmp/restapi.sock",
			},
			&cli.BoolFlag{
				Name:   define.FlagVerbose,
				Hidden: true,
				Value:  false,
			},
		},
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)
	if err := app.Run(ctx, os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	setLogrus(command)
	return ctx, nil
}

func showVersionAndOSInfo() error {
	var version strings.Builder
	if define.Version != "" {
		version.WriteString(define.Version)
	} else {
		version.WriteString("unknown")
	}

	if define.CommitID != "" {
		version.WriteString(define.CommitID)
	} else {
		version.WriteString(" (unknown)")
	}

	logrus.Infof("%s version: %s", os.Args[0], version.String())

	osInfo, err := system.GetOSVersion()
	if err != nil {
		return fmt.Errorf("failed to get os version: %w", err)
	}

	logrus.Infof("os version: %+v", osInfo)

	return nil
}

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

func setMaxMemory() uint64 {
	mb, err := system.GetMaxMemoryInMB()
	if err != nil {
		logrus.Warnf("failed to get max memory: %v", err)
		return 512
	}

	return mb
}

func createVMMProvider(ctx context.Context, command *cli.Command) (vm.Provider, error) {
	vmc := vmconfig.NewVMConfig()

	switch command.Name {
	case define.FlagRootfsMode:
		vmc.RunMode = define.RunUserRootfsMode
	case define.FlagDockerMode:
		vmc.RunMode = define.RunDockerEngineMode
	case define.FlagKernelMode:
		vmc.RunMode = define.RunKernelBootMode
	}

	vmc.WithResources(command.Uint64(define.FlagMemory), command.Int8(define.FlagCPUS))

	if vmc.RunMode == define.RunUserRootfsMode {
		if err := vmc.WithUserProvidedRootFS(command.String(define.FlagRootfs)); err != nil {
			return nil, fmt.Errorf("failed to set user provided rootfs: %w", err)
		}
	}

	if command.IsSet(define.FlagDiskDisk) {
		if err := vmc.WithUserProvidedDataDisk(command.StringSlice(define.FlagDiskDisk)); err != nil {
			return nil, fmt.Errorf("failed to set user provided data disk: %w", err)
		}
	}

	if command.IsSet(define.FlagMount) {
		if err := vmc.WithUserProvidedMounts(command.StringSlice(define.FlagMount)); err != nil {
			return nil, fmt.Errorf("failed to set user provided mounts: %w", err)
		}
	}

	vmc.WithUserProvidedCmdline(command.Args().First(), command.Args().Tail(), command.StringSlice("envs"))

	if command.IsSet(define.FlagRestAPIListenAddr) {
		if err := vmc.WithRESTAPIAddress(command.String(define.FlagRestAPIListenAddr)); err != nil {
			return nil, fmt.Errorf("failed to set rest api address: %w", err)
		}
	}

	if command.Name == define.FlagDockerMode {
		if err := vmc.WithBuiltInRootfs(); err != nil {
			return nil, fmt.Errorf("failed to use builtin rootfs: %w", err)
		}

		if err := vmc.WithContainerDataDisk(command.String(define.FlagContainerDataStorage)); err != nil {
			return nil, err
		}

		if err := vmc.WithPodmanListenAPIInHost(command.String(define.FlagListenUnixFile)); err != nil {
			return nil, err
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("can not get user home directry: %w", err)
		}

		if err = vmc.WithUserProvidedMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)}); err != nil {
			return nil, fmt.Errorf("failed to add user home mount point information: %w", err)
		}
	}

	// BUG: flock does not work on macOS after the system sleeps and wakes up
	_, err := vmc.Lock()
	if err != nil {
		return nil, err
	}

	if err = vmc.CreateRawDiskWhenNeeded(ctx); err != nil {
		return nil, fmt.Errorf("failed setup raw disk: %w", err)
	}

	if err = vmc.GenerateSSHInfo(); err != nil {
		return nil, err
	}

	if command.Bool(define.FlagUsingSystemProxy) {
		if err = vmc.TryGetSystemProxyAndSetToCmdline(); err != nil {
			return nil, err
		}
	}

	return vm.Get(vmc), nil
}
