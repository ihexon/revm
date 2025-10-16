package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startRootfs = cli.Command{
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
	if err := showVersionAndOSInfo(); err != nil {
		logrus.Warn("can not get Build version/OS information")
	}

	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	vmp, err := vmProviderFactory(ctx, define.RootFsMode, command)
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
			return server.NewAPIServer(vmc, server.RestAPIMode).Start(ctx)
		})
	}

	// Start service probers
	g.Go(func() error {
		return vmc.CloseChannelWhenServiceReady(ctx)
	})

	// Start Ignition server (no dependencies)
	g.Go(func() error {
		return server.NewAPIServer(vmc, server.IgnServerMode).Start(ctx)
	})

	g.Go(func() error {
		defer logrus.Debugf("vmp.StartNetwork(ctx) exit")
		return vmp.StartNetwork(ctx)
	})

	// VM Create and Start requires both GVProxy and IgnServer to be ready
	g.Go(func() error {
		defer logrus.Debugf("vmp.Start(ctx) exit")

		if err := vmc.WaitForServices(ctx, define.ServiceGVProxy, define.ServiceIgnServer); err != nil {
			return err
		}
		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}
