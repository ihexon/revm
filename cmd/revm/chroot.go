package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/eventreporter"
	"linuxvm/pkg/librevm"

	"github.com/urfave/cli/v3"
)

var startRootfs = cli.Command{
	Name:                      define.FlagChroot,
	Aliases:                   []string{"run"},
	Usage:                     "boot a Linux VM with a custom rootfs",
	UsageText:                 define.FlagChroot + " [flags] <command> [args...]",
	Description:               "boot a Linux microVM using libkrun and execute commands inside it, similar to chroot but with full kernel isolation",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		rootfsFlag,
		cpuFlag,
		memoryFlag,
		envsFlag,
		diskFlag,
		mountFlag,
		proxyFlag,
		workDirFlag,
		networkFlag,
		reportEventsFlag,
		logLevelFlag,
		logToFlag,
		sessionIDFlag,
		manageAPIFlag,
		sshKeyDirFlag,
		sshPrivateKeyFlag,
		sshPublicKeyFlag,
	},
	Action: rootfsLifeCycle,
}

func rootfsLifeCycle(_ context.Context, command *cli.Command) error {
	// 屏蔽上层 ctx，防止上游 ctx 导致 vm 意外退出
	// 如果要安全停止虚拟机，应该呼叫 cancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawDiskSpecs, err := librevm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
	if err != nil {
		return err
	}

	cfg := librevm.DefaultConfig().
		WithMode(librevm.ModeRootfs).
		WithName(command.String(define.FlagSessionID)).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithLogLevel(command.String(define.FlagLogLevel)).
		WithLogTo(command.String(define.FlagLogTo)).
		WithRootfs(command.String(define.FlagRootfs)).
		WithManageAPIFile(command.String(define.FlagManageAPIFile)).
		WithSSHKeyDir(command.String(define.FlagSSHKeyDir)).
		WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
		WithExportSSHKeyPublicFile(command.String(define.FlagExportSSHKeyPublicFile)).
		WithMount(command.StringSlice(define.FlagMount)...)

	cfg.WithRawDiskSpecs(rawDiskSpecs...)

	if command.Args().Len() > 0 {
		cfg.WithCommand(command.Args().First(), command.Args().Tail()...).
			WithWorkDir(command.String(define.FlagWorkDir)).
			WithEnv(command.StringSlice(define.FlagEnvs)...)
	}

	if u := command.String(define.FlagReportEvents); u != "" {
		cfg.WithEventReporter(eventreporter.NewV1(u, librevm.ModeRootfs))
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}
	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunChroot(ctx)
}
