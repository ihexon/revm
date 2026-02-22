package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
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
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "memory size in MB",
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

	event.Setup(command.String(define.FlagReportURL), event.Run)
	defer event.Emit(event.Exit)

	vmp, err := ConfigureVM(ctx, command, define.RootFsMode)
	if err != nil {
		return fmt.Errorf("configure vm fail: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	svc := service.NewHostServices(vmp)
	g.Go(func() error {
		return svc.ExitVirtualMachineWhenSomethingHappened(ctx)
	})

	g.Go(func() error {
		return svc.StartIgnitionService(ctx)
	})

	g.Go(func() error {
		return svc.StartNetworkStack(ctx)
	})

	g.Go(func() error {
		return svc.StartMachineManagementAPI(ctx)
	})

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vmp.GetVMConfigure().Readiness.VNetHostReady:
			return svc.StartVirtualMachine(ctx)
		}
	})

	errChan := make(chan error, 1)
	go func() { errChan <- g.Wait() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-errChan:
		if err != nil {
			event.EmitError(err)
		}
		return err
	}
}
