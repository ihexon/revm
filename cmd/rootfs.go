package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/httpserver"
	"linuxvm/pkg/service"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startRootfs = cli.Command{
	Name:        define.FlagRootfsMode,
	Usage:       "run a command in a Linux VM",
	UsageText:   define.FlagRootfsMode + " [flags] [command]",
	Description: "boot a Linux VM with the given rootfs and execute a command",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  define.FlagRootfs,
			Usage: "path to a custom rootfs directory",
		},
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of CPU cores",
			Value: setMaxCPUs(),
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "memory size in MB",
			Value: setMaxMemory(),
		},
		&cli.StringSliceFlag{
			Name:  define.FlagEnvs,
			Usage: "set environment variables, e.g. --envs=FOO=bar --envs=BAZ=qux",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "attach a raw disk image (auto-created if not exists)",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount a host directory into the guest (format: host:guest[:ro])",
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "forward host HTTP/HTTPS proxy settings to the guest",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkDir,
			Usage: "working directory for the command inside the guest",
			Value: "/",
		},
		&cli.StringFlag{
			Name:   define.FlagVNetworkType,
			Usage:  "network stack provider (gvisor, tsi)",
			Value:  string(define.GVISOR),
			Hidden: false,
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vmc.StopCh:
			return fmt.Errorf("VM stop requested via API")
		}
	})

	// start ign service
	ignSrv := httpserver.NewIgnServer(vmc)
	g.Go(func() error {
		return ignSrv.Start(ctx)
	})

	svc := service.NewHostServiceManager(vmc.VirtualNetworkMode)

	g.Go(func() error {
		return svc.StartNetworkStack(ctx, (*define.VMConfig)(vmc))
	})

	// start vmctl service
	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ignSrv.VNetHostReady:
			if err := vmp.Create(ctx); err != nil {
				return fmt.Errorf("failed to create vm: %w", err)
			}
			return vmp.Start(ctx)
		}
	})

	return g.Wait()
}
