package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"

	"github.com/urfave/cli/v3"
)

var startDocker = cli.Command{
	Name:                      define.FlagDockerMode,
	Usage:                     "start a Linux VM with the built-in container runtime",
	UsageText:                 define.FlagDockerMode + " [flags]",
	Description:               "boot a Linux microVM using libkrun with the built-in rootfs and podman container runtime; exposes a Podman-compatible API socket on the host",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1",
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagEnvs,
			Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]); auto-created if the file does not exist; new disks default to a random UUID and mount at /mnt/<UUID>; can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times",
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal",
		},
		&cli.StringFlag{
			Name:  define.FlagVNetworkType,
			Usage: "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception",
			Value: string(define.GVISOR),
		},
		&cli.StringFlag{
			Name:  define.FlagReportEvents,
			Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)",
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagLogTo,
			Usage: "custom log file path on host; defaults to /tmp/<session_id>/logs/vm.log when unset",
		},
		&cli.StringFlag{
			Name:  define.FlagSessionID,
			Usage: "session name; used to derive the workspace directory (/tmp/<session_id>); sessions with the same name are mutually exclusive via flock",
			Value: revm.RandomString(),
		},
		&cli.StringFlag{
			Name:  define.FlagContainerDisk,
			Usage: "persistent ext4 raw disk image for container storage (format: <path>[,version=<string>]); auto-created if missing; if the stored version xattr is missing or mismatched, the disk is recreated; defaults to a workspace-local disk with the built-in container disk version when unset",
		},
		&cli.StringFlag{
			Name:  define.FlagPodmanProxyAPIFile,
			Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock",
		},
		&cli.StringFlag{
			Name:  define.FlagManageAPIFile,
			Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock",
		},
		&cli.StringFlag{
			Name:  define.FlagExportSSHKeyPrivateFile,
			Usage: "file path to symlink the generated SSH key to",
		},
	},
	Action: dockerLifeCycle,
}

func dockerLifeCycle(_ context.Context, command *cli.Command) error {
	// 屏蔽上层 ctx，防止上游 ctx 导致 vm 意外退出
	// 如果要安全停止虚拟机，应该呼叫 cancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rawDiskSpecs, err := revm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
	if err != nil {
		return err
	}

	var containerDiskSpec *revm.ContainerDiskSpec
	if value := command.String(define.FlagContainerDisk); value != "" {
		spec, err := revm.ParseContainerDiskSpec(value)
		if err != nil {
			return err
		}
		containerDiskSpec = &spec
	}

	cfg := revm.DefaultConfig(command.String(define.FlagSessionID)).
		WithLogSetup(command.String(define.FlagLogLevel), command.String(define.FlagLogTo)).
		WithMode(revm.ModeContainer).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithMount(command.StringSlice(define.FlagMount)...).
		WithContainerDiskSpec(containerDiskSpec).
		WithPodmanProxyAPIFile(command.String(define.FlagPodmanProxyAPIFile)).
		WithManageAPIFile(command.String(define.FlagManageAPIFile)).
		WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
		WithRawDiskSpecs(rawDiskSpecs...)

	if u := command.String(define.FlagReportEvents); u != "" {
		cfg.WithEventReporter(u)
	}

	vm, err := revm.New(cfg)
	if err != nil {
		return err
	}

	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunDocker(ctx)
}
