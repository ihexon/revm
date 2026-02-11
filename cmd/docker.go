package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/httpserver"
	"linuxvm/pkg/system"

	"github.com/sirupsen/logrus"
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
			Value: int8(system.GetCPUCores()),
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
			Value:  "/tmp",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "workspace path",
			Value: fmt.Sprintf("/tmp/.revm-%s", FastRandomStr()),
		},
		&cli.StringFlag{
			Name:   define.FlagVNetworkType,
			Usage:  "network stack provider (GVISOR, TSI)",
			Value:  define.GVISOR.String(),
			Hidden: true,
		},
	},
	Action: dockerModeLifeCycle,
}

func dockerModeLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

	vmc, err := ConfigureVM(ctx, command, define.ContainerMode)
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

	// start virtual network service
	mode := vmc.GetNetworkMode()
	g.Go(func() error {
		return mode.StartNetworkStack(ctx, (*define.VMConfig)(vmc))
	})

	// start vmctl service
	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	// start podman proxy service
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ignSrv.VNetReady:
			return mode.StartPodmanProxy(ctx, (*define.VMConfig)(vmc))
		}
	})

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ignSrv.VNetReady:
			if err := vmp.Create(ctx); err != nil {
				return fmt.Errorf("create virtual machine from libkrun builder fail: %v", err)
			}
			return vmp.Start(ctx)
		}
	})

	// Wait for Podman API to be ready (via guest notification)
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ignSrv.PodmanReady:
			logrus.Infof("Podman API proxy listen in: %s", mode.GetPodmanListenAddr((*define.VMConfig)(vmc)))
			return nil
		}
	})

	return g.Wait()
}
