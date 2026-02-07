package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"

	"github.com/urfave/cli/v3"
)

var initCmd = cli.Command{
	Name: define.FlagInit,
	Usage: "Most of the functionality of [init] has been moved to [start]; " +
		"currently, [init] only focuses on whether the raw disk needs to be regenerated",
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "not used anymore",
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "not used anymore",
		},
		&cli.StringFlag{
			Name:  define.FlagName,
			Usage: "not used anymore",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagVolume,
			Usage: "not used anymore",
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
			Name:  define.FlagContainerRAWVersionXATTR,
			Usage: "control whether the container-disk.ext4 file is erased and regenerated",
			Value: "v1.0-ovm-containerStorage-ext4",
		},
	},
	Action: initAction,
}

// init 现在被弃用，这里只发送 event 信号，为了兼容
func initAction(ctx context.Context, command *cli.Command) error {
	return event.GetReporterFromCtx(ctx).SendEventInInitLifeCycle(ctx, event.Success, "")
}
