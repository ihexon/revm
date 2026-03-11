package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/eventreporter"
	"linuxvm/pkg/librevm"
	"os"

	"github.com/sirupsen/logrus"
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
		&cli.StringFlag{
			Name:  define.FlagContainerDisk,
			Usage: "path to a persistent ext4 raw disk image for container storage; auto-created if the file does not exist; defaults to a workspace-local disk if unset",
		},
		&cli.StringFlag{
			Name:  define.FlagPodmanProxyAPIFile,
			Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock",
		},
		manageAPIFlag,
		sshKeyDirFlag,
		sshPrivateKeyFlag,
		sshPublicKeyFlag,

		// legacy hidden flags set
		&cli.StringFlag{
			Name:   define.FlagOVMWorkspace,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.Uint64Flag{
			Name:   define.FlagOVMPPID,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   define.FlagOVMName,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   define.FlagOVMReportURL,
			Usage:  "legacy event, for ovm-js compatibility, use --report-events-to instead",
			Hidden: true,
		},
	},
	Action: dockerLifeCycle,
}

func dockerLifeCycle(_ context.Context, command *cli.Command) error {
	// 屏蔽上层 ctx，防止上游 ctx 导致 vm 意外退出
	// 如果要安全停止虚拟机，应该呼叫 cancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := librevm.DefaultConfig().
		WithMode(librevm.ModeContainer).
		WithName(command.String(define.FlagSessionID)).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithLogLevel(command.String(define.FlagLogLevel)).
		WithDisk(command.StringSlice(define.FlagRawDisk)...).
		WithMount(command.StringSlice(define.FlagMount)...)

	if cd := command.String(define.FlagContainerDisk); cd != "" {
		cfg.WithContainerDisk(cd)
	}

	if p := command.String(define.FlagPodmanProxyAPIFile); p != "" {
		cfg.WithPodmanProxyAPIFile(p)
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

	// if legacy event reporter is set, use it
	if u := command.String(define.FlagOVMReportURL); u != "" {
		cfg.WithEventReporter(eventreporter.NewLegacyReporter(u, librevm.ModeContainer))
	}

	if u := command.String(define.FlagReportEvents); u != "" {
		cfg.Reporters = nil
		cfg.WithEventReporter(eventreporter.NewV1(u, librevm.ModeContainer))
	}

	if l := command.String(define.FlagLogTo); l != "" {
		cfg.WithLogTo(l)
	}

	// Apply init vmconfig preferences if present.
	if initCfg, err := librevm.LoadFile(vmConfigFilePath); err == nil {
		logrus.Infof("[apply-vmconfig] apply vmconfig prefer from: %q", vmConfigFilePath)
		cfg.MergeFrom(initCfg)
		_ = os.Remove(vmConfigFilePath)
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}

	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunDocker(ctx)
}
