package main

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"linuxvm/pkg/libkrun"
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

	logrus.Infof("exec cmdline: %q", command.Args().Slice())
	cmdline := &vmconfig.Cmdline{
		TargetBin:     command.Args().First(),
		TargetBinArgs: command.Args().Tail(),
		Env:           command.StringSlice("envs"),
	}

	logrus.Infof("vmconfig: %+v", vmc)
	logrus.Infof("cmdline: %+v", cmdline)

	//return nil
	return libkrun.CreateVM(vmc, cmdline, libkrun.INFO)
}
