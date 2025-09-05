package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startDocker = cli.Command{
	Name:        define.FlagDockerMode,
	Aliases:     []string{"docker", "podman", "podman-mode", "container-mode", "container"},
	Usage:       "run in Docker-compatible mode",
	UsageText:   define.FlagDockerMode + " [OPTIONS] [command]",
	Description: "In Docker compatibility mode, the built-in Docker engine is used and a unix socks file is listened to as the API entry point used by the docker cli.",
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  "cpus",
			Usage: "given how many cpu cores",
			Value: int8(system.GetCPUCores()),
		},
		&cli.Int32Flag{
			Name:    "memory",
			Aliases: []string{"m"},
			Usage:   "given how many memory in MB",
			Value:   setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  "system-proxy",
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: false,
		},
		&cli.StringFlag{
			Name:     "rootfs",
			Aliases:  []string{"d"},
			Usage:    "path to Docker rootfs directory (must have Docker engine pre-installed)",
			Required: true,
		},
		&cli.StringFlag{
			Name:    define.FlagListenUnix,
			Aliases: []string{"l"},
			Usage:   "listen for Docker API requests on a Unix socket, forwarding them to the guest's Docker engine",
			Value:   define.DefaultPodmanAPIUnixSocksInHost,
		},
		&cli.StringSliceFlag{
			Name:    define.FlagDiskDisk,
			Aliases: []string{"O"},
			Usage:   "output all container data to the specified raw disk(a ext4 format image)",
		},
		&cli.StringSliceFlag{
			Name:  "mount",
			Usage: "mount host dir to guest dir",
		},
	},
	Action: dockerModeLifeCycle,
}

func dockerModeLifeCycle(ctx context.Context, command *cli.Command) error {
	vmp, err := createVMMProvider(ctx, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	if err := setDockerModeParameters(vmc, command); err != nil {
		return fmt.Errorf("failed to set docker mode parameters: %w", err)
	}

	rawDiskFile, err := filepath.Abs(command.StringSlice(define.FlagDiskDisk)[0])
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err = filesystem.CreateDiskAndFormatExt4(ctx, rawDiskFile, false); err != nil {
		return fmt.Errorf("failed to create raw disk: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return server.NewAPIServer(ctx, vmc).Start()
	})

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err := vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}

func setDockerModeParameters(vmc *vmconfig.VMConfig, command *cli.Command) error {
	// Set docker mode
	vmc.Cmdline.Mode = define.RunDockerEngineMode

	// Fill docker info
	path, err := filepath.Abs(command.String(define.FlagListenUnix))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   path,
	}

	vmc.PodmanInfo = define.PodmanInfo{
		UnixSocksAddr: unixAddr.String(),
	}

	// In docker-mode, we need to mount the host home directory to the guest home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("can not get user home directry: %w", err)
	}
	vmc.Mounts = append(vmc.Mounts, filesystem.CmdLineMountToMounts([]string{fmt.Sprintf("%s:%s", homeDir, homeDir)})...)

	return nil
}
