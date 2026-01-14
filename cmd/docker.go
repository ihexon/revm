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

	vmp, err := vmProviderFactory(ctx, define.ContainerMode, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	// Optional: Management API server for host-side control
	if command.IsSet(define.FlagRestAPIListenAddr) && command.String(define.FlagRestAPIListenAddr) != "" {
		g.Go(func() error {
			return server.NewManagementAPIServer(vmc).Start(ctx)
		})
	}

	// Service readiness prober
	g.Go(func() error {
		defer logrus.Debug("service prober exited")
		return vmc.CloseChannelWhenServiceReady(ctx)
	})

	// Guest config server (provides VM config to guest agent via VSock)
	g.Go(func() error {
		defer logrus.Debug("guest-config server exited")
		return server.NewGuestConfigServer(vmc).Start(ctx)
	})

	// Network backend (gvproxy)
	g.Go(func() error {
		defer logrus.Debug("network backend exited")
		return vmp.StartNetwork(ctx)
	})

	// VM lifecycle: wait for dependencies, then create and start
	g.Go(func() error {
		defer logrus.Debug("VM exited")

		if err := vmc.WaitForServices(ctx, define.ServiceGVProxy, define.ServiceIgnServer); err != nil {
			return err
		}

		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}

		return vmp.Start(ctx)
	})

	// Docker API tunnel: forward host Unix socket to guest Podman API
	g.Go(func() error {
		defer logrus.Debug("docker API tunnel exited")

		if err := vmc.WaitForServices(ctx, define.ServiceGVProxy); err != nil {
			return err
		}

		return network.TunnelHostUnixToGuest(ctx, vmc.GVproxyEndpoint, vmc.PodmanInfo.UnixSocksAddr, define.DefaultGuestAddr, uint16(define.DefaultGuestPodmanAPIPort))
	})

	return g.Wait()
}
