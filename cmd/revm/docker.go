package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/eventreporter"
	"linuxvm/pkg/librevm"

	"github.com/urfave/cli/v3"
)

var startDocker = cli.Command{
	Name:                      define.FlagDockerMode,
	Aliases:                   []string{"start"}, // for compatibility ovm-js ovm init
	Usage:                     "start a Linux VM with the built-in container runtime",
	UsageText:                 define.FlagDockerMode + " [flags]",
	Description:               "boot a Linux microVM using libkrun with the built-in rootfs and podman container runtime; exposes a Podman-compatible API socket on the host",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		cpuFlag,
		memoryFlag,
		envsFlag,
		diskFlag,
		mountFlag,
		proxyFlag,
		networkFlag,
		reportEventsFlag,
		logLevelFlag,
		logToFlag,
		sessionIDFlag,
		containerDiskFlag,
		podmanProxyAPIFileFlag,
		manageAPIFlag,
		sshKeyDirFlag,
		sshPrivateKeyFlag,
		sshPublicKeyFlag,
	},
	Action: dockerLifeCycle,
}

func dockerLifeCycle(_ context.Context, command *cli.Command) error {
	// 屏蔽上层 ctx，防止上游 ctx 导致 vm 意外退出
	// 如果要安全停止虚拟机，应该呼叫 cancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawDiskSpecs, err := librevm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
	if err != nil {
		return err
	}

	cfg := librevm.DefaultConfig().
		WithMode(librevm.ModeContainer).
		WithName(command.String(define.FlagSessionID)).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithLogLevel(command.String(define.FlagLogLevel)).
		WithLogTo(command.String(define.FlagLogTo)).
		WithMount(command.StringSlice(define.FlagMount)...).
		WithContainerDisk(command.String(define.FlagContainerDisk)).
		WithPodmanProxyAPIFile(command.String(define.FlagPodmanProxyAPIFile)).
		WithManageAPIFile(command.String(define.FlagManageAPIFile)).
		WithSSHKeyDir(command.String(define.FlagSSHKeyDir)).
		WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
		WithExportSSHKeyPublicFile(command.String(define.FlagExportSSHKeyPublicFile))

	cfg.WithRawDiskSpecs(rawDiskSpecs...)

	if u := command.String(define.FlagReportEvents); u != "" {
		cfg.Reporters = nil
		cfg.WithEventReporter(eventreporter.NewV1(u, librevm.ModeContainer))
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}

	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunDocker(ctx)
}
