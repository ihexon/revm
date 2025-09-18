package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/server"
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
			Name:    define.FlagMemory,
			Aliases: []string{"m"},
			Usage:   "given how many memory in MB",
			Value:   setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: false,
		},
		&cli.StringFlag{
			Name:    define.FlagRootfs,
			Aliases: []string{"d", "podman-rootfs"},
			Usage:   "path to another podman rootfs directory (must have podman pre-installed)",
		},
		&cli.StringFlag{
			Name:    define.FlagListenUnixFile,
			Aliases: []string{"l"},
			Usage:   "listen for Docker API requests on a Unix socket, forwarding them to the guest's Docker engine",
			Value:   define.DefaultPodmanAPIUnixSocksInHost,
		},
		&cli.StringFlag{
			Name:     define.FlagContainerDataStorage,
			Aliases:  []string{"data", "s", "save"},
			Usage:    "An raw data disk that save all container data",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount host dir to guest dir",
		},
	},
	Action: dockerModeLifeCycle,
}

func dockerModeLifeCycle(ctx context.Context, command *cli.Command) error {
	if err := showVersionAndOSInfo(); err != nil {
		logrus.Warn("cannot get Build version/OS information")
	}

	vmp, err := createVMMProvider(ctx, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	if command.IsSet(define.FlagRestAPIListenAddr) && command.String(define.FlagRestAPIListenAddr) != "" {
		g.Go(func() error {
			return server.NewAPIServer(vmc).Start(ctx)
		})
	}

	g.Go(func() error {
		return server.IgnProvisionerServer(ctx, vmc, vmc.IgnProvisionerAddr)
	})

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}

		vmc.WaitIgnServerReady(ctx)
		vmc.WaitGVProxyReady(ctx)

		return vmp.Start(ctx)
	})

	g.Go(func() error {
		tcpAddr, err := network.ParseTcpAddr(define.PodmanDefaultListenTcpAddrInGuest)
		if err != nil {
			return fmt.Errorf("failed to parse tcp addr %q: %w", define.PodmanDefaultListenTcpAddrInGuest, err)
		}

		vmc.WaitGVProxyReady(ctx)
		return network.ForwardPodmanAPIOverVSock(ctx, vmc.GVproxyEndpoint, vmc.PodmanInfo.UnixSocksAddr, tcpAddr.Host, uint16(tcpAddr.Port))
	})

	return g.Wait()
}
