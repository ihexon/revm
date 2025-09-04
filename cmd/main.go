//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run a linux shell in 1 second",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "run a linux shell in 1 second",
		Before:                    earlyStage,
		DisableSliceFlagSeparator: true,
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startVM,
		&startDocker,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
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

		PodmanInfo: define.PodmanInfo{
			PodmanAPITcpAddressInHost: define.DefaultPodmanTcpAddressInHost,
			PodmanAPITcpAddressInVM:   define.DefaultPodmanTcpAddressInVM,
			PodmanAPITcpPortInHost:    define.DefaultPodmanTcpPortInHost,
			PodmanAPITcpPortInVM:      define.DefaultPodmanTcpPortInVM,
		},
	}

	return &vmc
}
