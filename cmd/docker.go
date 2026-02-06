package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/service"
	"linuxvm/pkg/system"

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
			Name:  define.FlagCPUS,
			Usage: "given how many cpu cores",
			Value: int8(system.GetCPUCores()),
		},
		&cli.Uint64Flag{
			Name:    define.FlagMemoryInMB,
			Aliases: []string{"m"},
			Usage:   "given how many memory in MB",
			Value:   setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: false,
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "attach another raw disk into guest",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount another host dir to guest",
		},
		&cli.StringFlag{
			Name:   define.FlagWorkDir,
			Usage:  "set cmdline workdir",
			Hidden: true,
			Value:  "/tmp",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "workspace path",
			Value: fmt.Sprintf("/tmp/.revm-%s", FastRandomStr()),
		},
		&cli.StringFlag{
			Name:  define.FlagVNetworkType,
			Usage: "network stack provider (gvisor,TSI)",
			Value: define.GVISOR.String(),
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

	g.Go(func() error {
		return vmp.StartIgnServer(ctx)
	})

	// Start network stack based on mode
	mode := vmc.GetNetworkMode()
	g.Go(func() error {
		return mode.StartNetworkStack(ctx, (*define.VMConfig)(vmc))
	})

	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	// Start Podman API proxy based on mode
	g.Go(func() error {
		return mode.StartPodmanProxy(ctx, (*define.VMConfig)(vmc))
	})

	g.Go(func() error {
		if err := service.WaitAll(ctx, service.NewIgnServerProbe(vmc.IgnitionServerCfg.ListenSockAddr)); err != nil {
			return err
		}

		// Wait for network stack to be ready
		if err := mode.WaitNetworkReady(ctx, (*define.VMConfig)(vmc)); err != nil {
			return err
		}

		if err := vmp.Create(ctx); err != nil {
			return fmt.Errorf("create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	// Wait for Podman API to be ready
	g.Go(func() error {
		return mode.WaitPodmanReady(ctx, (*define.VMConfig)(vmc))
	})

	return g.Wait()
}
