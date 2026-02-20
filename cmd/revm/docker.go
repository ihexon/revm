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

var startDocker = cli.Command{
	Name:        define.FlagDockerMode,
	Usage:       "run in Docker-compatible mode",
	UsageText:   define.FlagDockerMode + " [OPTIONS] [command]",
	Description: "In Docker-compatible mode, the built-in Podman engine is used and a Unix socket is exposed as the API endpoint for the docker/podman CLI.",
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of CPU cores",
			Value: setMaxCPUs(),
		},
		&cli.Uint64Flag{
			Name:    define.FlagMemoryInMB,
			Aliases: []string{"m"},
			Usage:   "memory size in MB",
			Value:   setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "forward host HTTP/HTTPS proxy settings to the container engine",
			Value: false,
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "attach a raw disk image to the guest",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount a host directory into the guest",
		},
		&cli.StringFlag{
			Name:   define.FlagWorkDir,
			Usage:  "working directory for the command inside the guest",
			Hidden: true,
			Value:  "/",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "workspace path",
			Value: fmt.Sprintf("/tmp/.revm-%s", FastRandomStr()),
		},
		&cli.StringFlag{
			Name:   define.FlagVNetworkType,
			Usage:  "network stack provider (gvisor, tsi)",
			Value:  string(define.GVISOR),
			Hidden: false,
		},
	},
	Action: dockerModeLifeCycle,
}

func dockerModeLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

	event.Setup(command.String(define.FlagReportURL), event.Docker)
	defer event.Emit(event.Exit)

	vm, err := ConfigureVM(ctx, command, define.ContainerMode)
	if err != nil {
		return fmt.Errorf("configure vm fail: %w", err)
	}

	vmp, err := GetVMM(vm)
	if err != nil {
		return fmt.Errorf("get vmm fail: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return service.ListenSignal(ctx)
	})

	g.Go(func() error {
		return service.WatchParentProcess(ctx)
	})

	g.Go(func() error {
		return service.WatchMachineExitChannel(ctx, (*define.Machine)(vm))
	})

	g.Go(func() error {
		return service.StartIgnitionService(ctx, (*define.Machine)(vm))
	})

	g.Go(func() error {
		return service.StartNetworkStack(ctx, (*define.Machine)(vm))
	})

	// start vmctl service
	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-(*define.Machine)(vm).Readiness.VNetHostReady:
			if err := vmp.Create(ctx); err != nil {
				return fmt.Errorf("create virtual machine from libkrun builder fail: %v", err)
			}
			return vmp.Start(ctx)
		}
	})

	// start podman proxy service
	g.Go(func() error {
		return service.StartPodmanAPIProxy(ctx, (*define.Machine)(vm))
	})

	if err = g.Wait(); err != nil {
		event.EmitError(err)
		return err
	}

	return nil
}
