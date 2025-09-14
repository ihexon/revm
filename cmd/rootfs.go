package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startVM = cli.Command{
	Name:        define.FlagRootfsMode,
	Aliases:     []string{"rootfs", "run"},
	Usage:       "run the rootfs",
	UsageText:   define.FlagRootfsMode + " [flags] [command]",
	Description: "run any rootfs with the given command",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     define.FlagRootfs,
			Usage:    "rootfs path, e.g. /var/lib/libkrun/rootfs/alpine-3.15.0",
			Required: true,
		},
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "given how many cpu cores",
			Value: int8(system.GetCPUCores()),
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemory,
			Usage: "set memory in MB",
			Value: setMaxMemory(),
		},
		&cli.StringSliceFlag{
			Name:  define.FlagEnvs,
			Usage: "set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux",
		},
		&cli.StringSliceFlag{
			Name:    define.FlagDiskDisk,
			Aliases: []string{"disk"},
			Usage:   "attach one or more data disk and automount into /var/tmp/data_disk/<UUID>",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount host dir to guest dir",
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to guest",
			Value: false,
		},
	},
	Action: rootfsLifeCycle,
}

func rootfsLifeCycle(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	vmp, err := createVMMProvider(ctx, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	if command.IsSet(define.FlagRestAPIListenAddr) && command.String(define.FlagRestAPIListenAddr) != "" {
		g.Go(func() error {
			return server.NewAPIServer(vmc).Start(ctx)
		})
	}

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}
