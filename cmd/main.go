//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	app := cli.Command{
		Name:        os.Args[0],
		Usage:       "run a linux shell in 1 second",
		UsageText:   os.Args[0] + " [command] [flags]",
		Description: "run a linux shell in 1 second",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "rootfs",
				Usage:    "rootfs path, e.g. /var/lib/libkrun/rootfs/alpine-3.15.0",
				Required: true,
			},
			&cli.Int8Flag{
				Name:  "cpus",
				Usage: "given how many cpu cores",
				Value: int8(system.GetCPUCores()),
			},
			&cli.Int32Flag{
				Name:  "memory",
				Usage: "set memory in MB",
				Value: setMaxMemory(),
			},
			&cli.StringSliceFlag{
				Name:  "envs",
				Usage: "set envs for cmdline, e.g. --envs=FOO=bar --envs=BAZ=qux",
			},
			&cli.StringSliceFlag{
				Name:  "data-disk",
				Usage: "set data disk path, the disk will be map into /dev/vdX",
			},
			&cli.StringSliceFlag{
				Name:  "mount",
				Usage: "mount host dir to guest dir",
			},
			&cli.BoolFlag{
				Name:  "system-proxy",
				Usage: "use system proxy, set environment http(s)_proxy to guest",
				Value: false,
			},
		},
		Action: vmLifeCycle,
	}

	app.DisableSliceFlagSeparator = true

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func setMaxMemory() int32 {
	mb, err := system.GetMaxMemoryInMB()
	if err != nil {
		logrus.Warnf("failed to get max memory: %v", err)
		return 512
	}

	return int32(mb)
}

func vmLifeCycle(ctx context.Context, command *cli.Command) error {
	setLogrus()
	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set rlimit: %v", err)
	}

	vmc := makeVMCfg(command)
	d, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %v", err)
	}
	logrus.Infof("vmconfig: %s", d)

	cmdline := makeCmdline(command)

	if command.Bool("system-proxy") {
		if err := cmdline.UsingSystemProxy(); err != nil {
			return fmt.Errorf("failed to use system proxy: %v", err)
		}
	}
	_ = cmdline.SetPATH()

	d, err = json.Marshal(cmdline)
	if err != nil {
		return fmt.Errorf("failed to marshal cmdline: %v", err)
	}

	logrus.Infof("revm cmdline: %s", d)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return server.NewServer(ctx, vmc).Start()
	})

	vmp := vm.Get()

	g.Go(func() error {
		return vmp.StartNetwork(ctx, vmc)
	})

	g.Go(func() error {
		if err = vmp.Create(ctx, vmc, cmdline); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}

		return vmp.Start(ctx)
	})

	return g.Wait()
}

func makeVMCfg(command *cli.Command) *vmconfig.VMConfig {
	prefix := filepath.Join(os.TempDir(), uuid.New().String()[:8])
	vmc := vmconfig.VMConfig{
		MemoryInMB:          command.Int32("memory"),
		Cpus:                command.Int8("cpus"),
		RootFS:              command.String("rootfs"),
		DataDisk:            command.StringSlice("data-disk"),
		Mounts:              filesystem.CmdLineMountToMounts(command.StringSlice("mount")),
		GVproxyEndpoint:     fmt.Sprintf("unix://%s/gvproxy-control.sock", prefix),
		NetworkStackBackend: fmt.Sprintf("unixgram://%s/gvproxy-network-backend.sock", prefix),
	}

	return &vmc
}

func makeCmdline(command *cli.Command) *vmconfig.Cmdline {
	cmdline := vmconfig.Cmdline{
		Workspace:     "/",
		TargetBin:     "/3rd/bootstrap",
		TargetBinArgs: append([]string{command.Args().First()}, command.Args().Tail()...),
		Env:           command.StringSlice("envs"),
	}
	return &cmdline
}

func setLogrus() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logrus.SetOutput(os.Stderr)
	logrus.SetLevel(logrus.InfoLevel)
}
