package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig"

	"github.com/urfave/cli/v3"
)

func ConfigureVM(ctx context.Context, command *cli.Command, runMode define.RunMode) error {
	_, err := vmconfig.NewVMConfigBuilder(runMode).
		SetWorkspace(command.String(define.FlagWorkspace)).
		SetLogLevel(command.String(define.FlagLogLevel)).
		SetResources(command.Int8(define.FlagCPUS), command.Uint64(define.FlagMemoryInMB)).
		SetNetworkMode(define.String2NetworkMode(command.String(define.FlagVNetworkType))).
		SetUsingSystemProxy(command.Bool(define.FlagUsingSystemProxy)).
		SetContainerDiskVersion(command.String(define.FlagContainerRAWVersionXATTR)).
		SetMounts(command.StringSlice(define.FlagMount)).
		Build(ctx)
	return err
}
