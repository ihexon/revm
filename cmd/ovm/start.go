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
	Description: "In Docker-compatible mode, the built-in Podman engine is used and a Unix socket is exposed as the API endpoint for the docker/podman CLI.",
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of CPU cores",
			Value: int8(system.GetCPUCores()),
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "memory size in MB",
			Value: 512,
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "forward host HTTP/HTTPS proxy settings to the Podman engine",
			Value: true,
		},
		&cli.StringFlag{
			Name:  define.FlagName,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagVolume,
			Usage: "mount a host directory into the guest",
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
			Usage: "log verbosity (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagContainerRAWVersionXATTR,
			Usage: "control whether the container-disk.ext4 file is erased and regenerated",
		},
		&cli.StringFlag{
			Name:  define.FlagVNetworkType,
			Usage: "network stack provider (GVISOR, TSI)",
			Value: string(define.GVISOR),
		},
	},
	Action: ovmLifeCycle,
}

func ovmLifeCycle(ctx context.Context, command *cli.Command) error {
	// Configure all parameters required by the OVM virtual machine
	if err := ConfigureVM(ctx, command, define.OVMode); err != nil {
		return err
	}

	return nil
}
