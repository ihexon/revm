package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"

	"github.com/urfave/cli/v3"
)

var startCmd = cli.Command{
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
			Name:  define.FlagMemoryInMB,
			Usage: "given how many memory in MB",
			Value: 512,
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: true,
		},
		&cli.StringFlag{
			Name:  define.FlagName,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagVolume,
			Usage: "mount host dir to guest",
		},
		&cli.StringFlag{
			Name:  define.FlagPPID,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagBoot,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagBootVersion,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "set log level, the logs will output in console, and also write into workspace log directory",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagContainerRAWVersionXATTR,
			Usage: "control whether the container-disk.ext4 file is erased and regenerated",
		},
		&cli.StringFlag{
			Name:  define.FlagVNetworkType,
			Usage: "set virtual network mode",
			Value: define.GVISOR.String(),
		},
	},
	Action: ovmLifeCycle,
}

func ovmLifeCycle(ctx context.Context, command *cli.Command) error {
	// 配置 ovm 虚拟机需要的所有参数
	if err := ConfigureVM(ctx, command, define.OVMode); err != nil {
		return err
	}

	return nil
}
