package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startVM = cli.Command{
	Name:        "run",
	Usage:       "run the rootfs",
	UsageText:   "run [flags] [command]",
	Description: "run any rootfs with the given command",
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

func setMaxMemory() int32 {
	mb, err := system.GetMaxMemoryInMB()
	if err != nil {
		logrus.Warnf("failed to get max memory: %v", err)
		return 512
	}

	return int32(mb)
}

func createVMMProvider(ctx context.Context, command *cli.Command) (vm.Provider, error) {
	vmc := makeVMCfg(command)

	_, err := vmc.Lock()
	if err != nil {
		return nil, err
	}

	if err = vmc.GenerateSSHInfo(); err != nil {
		return nil, err
	}

	if command.Bool("system-proxy") {
		if err = vmc.TryGetSystemProxyAndSetToCmdline(); err != nil {
			return nil, err
		}
	}

	return vm.Get(vmc), nil
}

func vmLifeCycle(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	vmp, err := createVMMProvider(ctx, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		vmc, err := vmp.GetVMConfigure()
		if err != nil {
			return err
		}
		return server.NewAPIServer(ctx, vmc).Start()
	})

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	return g.Wait()
}

func makeVMCfg(command *cli.Command) *vmconfig.VMConfig {
	prefix := filepath.Join(os.TempDir(), system.GenerateRandomID())

	vmc := vmconfig.VMConfig{
		MemoryInMB: command.Int32("memory"),
		Cpus:       command.Int8("cpus"),
		RootFS:     command.String("rootfs"),
		DataDisk:   command.StringSlice("data-disk"),
		Mounts:     filesystem.CmdLineMountToMounts(command.StringSlice("mount")),

		GVproxyEndpoint:     fmt.Sprintf("unix://%s/%s", prefix, define.GvProxyControlEndPoint),
		NetworkStackBackend: fmt.Sprintf("unixgram://%s/%s", prefix, define.GvProxyNetworkEndpoint),
		SSHInfo: define.SSHInfo{
			HostSSHKeyPairFile: filepath.Join(prefix, define.SSHKeyPair),
		},

		Cmdline: define.Cmdline{
			Bootstrap:     define.BootstrapBinary,
			BootstrapArgs: []string{},
			Workspace:     define.DefalutWorkDir,
			Mode:          define.RunUserCommandLineMode,
			TargetBin:     command.Args().First(),
			TargetBinArgs: command.Args().Tail(),
			Env:           append(command.StringSlice("envs"), define.DefaultPATH),
		},
	}

	logrus.Infof("cmdline: %q, %q", vmc.Cmdline.TargetBin, vmc.Cmdline.TargetBinArgs)

	return &vmc
}
