package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/probes"
	"linuxvm/pkg/system"

	"github.com/sirupsen/logrus"
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
		return vmp.StartIgnServer(ctx)
	})

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		return vmp.StartVMCtlServer(ctx)
	})

	g.Go(func() error {
		if err := probes.WaitAll(ctx,
			probes.NewGVProxyProbe(vmc.GvisorTapVsockEndpoint),
			probes.NewIgnServerProbe(vmc.IgnitionServerCfg.ListenUnixSockAddr),
		); err != nil {
			return err
		}

		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	g.Go(func() error {
		if err := probes.WaitAll(ctx,
			probes.NewGVProxyProbe(vmc.GvisorTapVsockEndpoint),
		); err != nil {
			return err
		}

		return gvproxy.TunnelHostUnixToGuest(ctx,
			vmc.GvisorTapVsockEndpoint,
			vmc.PodmanInfo.LocalPodmanProxyAddr,
			vmc.PodmanInfo.GuestPodmanAPIIP,
			vmc.PodmanInfo.GuestPodmanAPIPort)
	})

	g.Go(func() error {
		if err = probes.WaitAll(ctx,
			probes.NewPodmanProbe(vmc.PodmanInfo.LocalPodmanProxyAddr),
		); err != nil {
			return err
		}
		logrus.Infof("podman api proxy listen in: %q", vmc.PodmanInfo.LocalPodmanProxyAddr)
		return nil
	})

	err = g.Wait()
	return err
}
