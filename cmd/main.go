package main

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig"
	"os"
)

func main() {
	app := cli.Command{
		Name:        "alpiner",
		Usage:       "Linux VM",
		UsageText:   "linuxvm [command] [flags]",
		Description: "Linux VM",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "rootfs",
				Required: true,
			},
			&cli.Int8Flag{
				Name:  "cpus",
				Value: 1,
			},
			&cli.Int32Flag{
				Name:  "memory",
				Value: 512,
			},
			&cli.StringSliceFlag{
				Name: "envs",
			},
		},
		Action: CreateVM,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func CreateVM(ctx context.Context, command *cli.Command) error {
	vmc := &vmconfig.VMConfig{
		MemoryInMB: command.Int32("memory"),
		Cpus:       command.Int8("cpus"),
		RootFS:     command.String("rootfs"),
	}

	tmpdir, err := os.MkdirTemp("", "gvproxy")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %v", err)
	}

	vmc.GVproxyEndpoint = fmt.Sprintf("unix://%s/gvproxy-control.sock", tmpdir)
	vmc.NetworkStackBackend = fmt.Sprintf("unixgram://%s/vfkit-network-backend.sock", tmpdir)

	logrus.Warnf("%v", command.Args().First())

	cmdline := &vmconfig.Cmdline{
		TargetBin:     command.Args().First(),
		TargetBinArgs: command.Args().Tail(),
		Env:           command.StringSlice("envs"),
	}

	logrus.Infof("set memory to: %v", vmc.MemoryInMB)
	logrus.Infof("set cpus to: %v", vmc.Cpus)
	logrus.Infof("set rootfs to: %v", vmc.RootFS)
	logrus.Infof("set gvproxy control: %q", vmc.GVproxyEndpoint)
	logrus.Infof("set network backend: %q", vmc.NetworkStackBackend)
	logrus.Infof("set envs: %v", cmdline.Env)
	logrus.Infof("run cmdline: %v, %v", cmdline.TargetBin, cmdline.TargetBinArgs)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return network.StartNetworking(ctx, vmc)
	})

	g.Go(func() error {
		return libkrun.CreateVM(ctx, vmc, cmdline, libkrun.INFO)
	})

	return g.Wait()
}
