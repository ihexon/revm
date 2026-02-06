package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/service"

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
			Name:  define.FlagRootfs,
			Usage: "your custom rootfs path",
		},
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "given how many cpu cores",
			Value: 1,
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "set memory in MB",
			Value: 512,
		},
		&cli.StringSliceFlag{
			Name:  define.FlagEnvs,
			Usage: "set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "create/attach one or more data disk and automount into guest",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount host dir to guest dir",
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to guest",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkDir,
			Usage: "set cmdline workdir in rootfs",
			Value: "/",
		},
		&cli.StringFlag{
			Name:  define.FlagNetwork,
			Usage: "network stack provider (gvisor,TSI)",
			Value: string(define.GvisorNet),
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "workspace path",
			Value: fmt.Sprintf("/tmp/.revm-%s", FastRandomStr()),
		},
	},
	Action: rootfsLifeCycle,
}

func rootfsLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	vmc, err := ConfigureVM(ctx, command, define.RootFsMode)
	if err != nil {
		return fmt.Errorf("configure vm fail: %w", err)
	}

	vmp, err := GetVMM(vmc)
	if err != nil {
		return fmt.Errorf("get vmm fail: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return vmp.StartIgnServer(ctx)
	})

	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err = service.WaitAll(ctx,
			service.NewIgnServerProbe(vmc.IgnitionServerCfg.ListenSockAddr),
		); err != nil {
			return err
		}

		// TSI mode doesn't need gvisor-tap-vsock service
		if !vmc.TSI {
			if err = service.WaitAll(ctx,
				service.NewGVProxyProbe(vmc.GVPCtlAddr),
			); err != nil {
				return err
			}
		}

		if err := vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}
