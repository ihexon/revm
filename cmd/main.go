//go:build (darwin && arm64) || (linux && (arm64 || amd64))

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
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
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
		Before: earlyStage,
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
	if command.Args().Len() < 1 {
		return fmt.Errorf("no command specified")
	}

	if err := system.Rlimit(); err != nil {
		return fmt.Errorf("failed to set rlimit: %v", err)
	}

	vmc := makeVMCfg(command)

	// Lock the rootfs, only one vm instance can use it.
	fileLock := flock.New(filepath.Join(vmc.RootFS, ".lock"))
	logrus.Infof("lock rootfs: %s", vmc.RootFS)
	ifLocked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to lock rootfs: %w", err)
	}

	defer func() {
		if ifLocked {
			if err := fileLock.Unlock(); err != nil {
				logrus.Errorf("failed to unlock rootfs: %v", err)
			}
		}
	}()

	if !ifLocked {
		return fmt.Errorf("failed to lock rootfs, mybe there is another vm instance running")
	}

	if err := vmc.GenerateSSHInfo(); err != nil {
		return err
	}

	cmdline := makeCmdline(command)
	if command.Bool("system-proxy") {
		if err := cmdline.TryGetSystemProxyAndSetToCmdline(); err != nil {
			return err
		}
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return server.NewServer(ctx, vmc).Start()
	})

	vmp := vm.Get(vmc)

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err := vmp.Create(ctx, cmdline); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}
		return vmp.Start(ctx)
	})

	g.Go(func() error {
		return vmp.SyncTime(ctx)
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
		GVproxyEndpoint:     fmt.Sprintf("unix://%s/%s", prefix, define.GvProxyControlEndPoint),
		NetworkStackBackend: fmt.Sprintf("unixgram://%s/%s", prefix, define.GvProxyNetworkEndpoint),
		HostSSHKeyPair:      filepath.Join(os.TempDir(), uuid.New().String()[:8], define.SSHKeyPair),
	}

	return &vmc
}

func makeCmdline(command *cli.Command) *vmconfig.Cmdline {
	cmdline := vmconfig.Cmdline{
		Workspace:     define.DefalutWorkDir,
		TargetBin:     define.BootstrapBinary,
		TargetBinArgs: append([]string{command.Args().First()}, command.Args().Tail()...),
		Env:           append(command.StringSlice("envs"), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/3rd"),
	}

	return &cmdline
}

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	setLogrus()
	ctx, _ = signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT, os.Interrupt)

	return ctx, nil
}

func setLogrus() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logrus.SetOutput(os.Stderr)
	logrus.SetLevel(logrus.InfoLevel)
}
