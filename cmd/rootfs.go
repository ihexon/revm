package main

import (
	"context"
	"fmt"
	"time"

	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/probes"
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

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	// ========= Initialize service probes ============
	addr, err := network.ParseUnixAddr(vmc.GVproxyEndpoint)
	if err != nil {
		return err
	}
	gvProxyServiceProbe := probes.NewGVProxyService(addr.Path)
	sshServiceProbe := probes.NewGuestSSHService(addr.Path, vmc.SSHInfo.HostSSHKeyPairFile)

	addr, err = network.ParseUnixAddr(vmc.VMConfigProvisionerAddr)
	if err != nil {
		return err
	}
	vmCfgProvisionerProbe := probes.NewVMConfigProvisionerServer(addr.Path)

	g, ctx := errgroup.WithContext(ctx)

	// Optional: Management API server for host-side control
	if command.IsSet(define.FlagRestAPIListenAddr) && command.String(define.FlagRestAPIListenAddr) != "" {
		g.Go(func() error {
			defer logrus.Infof("management API server exited")
			return server.NewManagementAPIServer(vmc).Start(ctx)
		})
	}

	// Service readiness prober
	g.Go(func() error {
		defer logrus.Infof("service prober exited")

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		g, ctx := errgroup.WithContext(ctx)

		g.Go(func() error {
			return gvProxyServiceProbe.ProbeUntilReady(ctx)
		})

		g.Go(func() error {
			return vmCfgProvisionerProbe.ProbeUntilReady(ctx)
		})

		g.Go(func() error {
			return sshServiceProbe.ProbeUntilReady(ctx)
		})

		if err := g.Wait(); err != nil {
			return fmt.Errorf("failed to probe services: %w", err)
		}

		return nil
	})

	// Guest config server (provides VM config to guest agent via VSock)
	g.Go(func() error {
		defer logrus.Infof("guest-config server exited")
		return server.NewGuestConfigServer(vmc).Start(ctx)
	})

	// Network backend (gvproxy)
	g.Go(func() error {
		defer logrus.Infof("network backend exited")
		return vmp.StartNetwork(ctx)
	})

	// VM lifecycle: wait for dependencies, then create and start
	g.Go(func() error {
		defer logrus.Infof("VM exited")

		gvProxyServiceProbe.WaitUntilReady(ctx)
		vmCfgProvisionerProbe.WaitUntilReady(ctx)

		if err := vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}
