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
		&cli.StringFlag{
			Name:  define.FlagRootfs,
			Usage: "path to a rootfs directory to use as the VM root filesystem; must contain /bin/sh; takes priority over the built-in rootfs",
		},
		cpuFlag,
		memoryFlag,
		envsFlag,
		diskFlag,
		mountFlag,
		proxyFlag,
		&cli.StringFlag{
			Name:  define.FlagWorkDir,
			Usage: "working directory for command execution inside the guest; the guest-agent chdirs to this path before running the command",
			Value: "/",
		},
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
	cfg := librevm.DefaultConfig().
		WithMode(librevm.ModeRootfs).
		WithName(command.String(define.FlagSessionID)).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithLogLevel(command.String(define.FlagLogLevel)).
		WithDisk(command.StringSlice(define.FlagRawDisk)...).
		WithMount(command.StringSlice(define.FlagMount)...)

	if r := command.String(define.FlagRootfs); r != "" {
		cfg.WithRootfs(r)
	}

	if command.Args().Len() > 0 {
		cfg.WithCommand(command.Args().First(), command.Args().Tail()...).
			WithWorkDir(command.String(define.FlagWorkDir)).
			WithEnv(command.StringSlice(define.FlagEnvs)...)
	}

	if u := command.String(define.FlagReportEvents); u != "" {
		cfg.WithEventReporter(eventreporter.NewV1(u, librevm.ModeRootfs))
	}
	if l := command.String(define.FlagLogTo); l != "" {
		cfg.WithLogTo(l)
	}
	if m := command.String(define.FlagManageAPIFile); m != "" {
		cfg.WithManageAPIFile(m)
	}
	if sk := command.String(define.FlagSSHKeyDir); sk != "" {
		cfg.WithSSHKeyDir(sk)
	}
	if pk := command.String(define.FlagExportSSHKeyPrivateFile); pk != "" {
		cfg.WithExportSSHKeyPrivateFile(pk)
	}
	if pub := command.String(define.FlagExportSSHKeyPublicFile); pub != "" {
		cfg.WithExportSSHKeyPublicFile(pub)
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}
	vm.Cancel = cancel

	defer vm.Close()

	return vm.Run(ctx)
}
