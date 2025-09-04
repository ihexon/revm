package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"
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
			Name:  "memory",
			Usage: "given how many memory in MB",
			Value: setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  "system-proxy",
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: false,
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
	vmc.Cmdline.Mode = define.RunDockerEngineMode

	dir, err := system.Get3rdDir()
	if err != nil {
		return fmt.Errorf("failed to get 3rd dir: %w", err)
	}
	vmc.RootFS = filepath.Join(dir, "linux", define.BuiltinRootfsDirName)

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
