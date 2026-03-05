package main

import (
	"context"
	"linuxvm/pkg/define"

	"github.com/urfave/cli/v3"
)

var runCmd = cli.Command{
	Name:   define.FlagOVMStart,
	Action: runAction,
}

// 暂时未实现，不需要审查
func runAction(ctx context.Context, command *cli.Command) error {

	return nil
}
