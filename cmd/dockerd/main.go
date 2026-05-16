//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:                      "dockerd",
		Usage:                     "start a Linux VM with the built-in container runtime",
		UsageText:                 "dockerd [flags]\n   dockerd --attach --id <session-id> [--pty] [-- <command> [args...]]",
		Description:               "boot a Linux microVM using libkrun with the built-in rootfs and podman container runtime; exposes a Podman-compatible API socket on the host; use --attach to connect to an existing session",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512"},
			&cli.BoolFlag{Name: define.FlagAttachMode, Usage: "attach to an existing VM session instead of booting a new VM; requires --id"},
			&cli.BoolFlag{Name: define.FlagPTY, Usage: "allocate a pseudo-terminal when attaching and launch an interactive shell"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]); auto-created if the file does not exist; new disks default to a random UUID and mount at /mnt/<UUID>; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagReportEvents, Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)"},
			&cli.StringFlag{Name: define.FlagLogLevel, Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)", Value: "info"},
			&cli.StringFlag{Name: define.FlagLogTo, Usage: "custom log file path on host; defaults to /tmp/<session_id>/logs/vm.log when unset"},
			&cli.StringFlag{Name: define.FlagSessionID, Usage: "required session name; used to derive the workspace directory; sessions with the same name are mutually exclusive via flock", Required: true},
			&cli.StringFlag{Name: define.FlagContainerDisk, Usage: "persistent ext4 raw disk image for container storage (format: <path>[,version=<string>]); auto-created if missing; if the stored version xattr is missing or mismatched, the disk is recreated; defaults to a workspace-local disk with the built-in container disk version when unset"},
			&cli.StringFlag{Name: define.FlagPodmanProxyAPIFile, Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock"},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock"},
			&cli.StringFlag{Name: define.FlagExportSSHKeyPrivateFile, Usage: "file path to symlink the generated SSH key to"},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			ctx := context.Background()

			cfg := revm.DefaultConfig().
				WithSessionID(command.String(define.FlagSessionID)).
				WithLogging(command.String(define.FlagLogLevel), command.String(define.FlagLogTo)).
				WithPTY(command.Bool(define.FlagPTY))

			if command.Bool(define.FlagAttachMode) {
				cfg.WithAttach(command.Args().Slice()...)
			} else {
				cfg.WithMode(revm.ModeContainer).
					WithCommandLine(command.Args().Slice()...)
			}

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

			cfg.
				WithCPUs(int(command.Int8(define.FlagCPUS))).
				WithMemory(command.Uint64(define.FlagMemoryInMB)).
				WithNetwork(string(define.GVISOR)).
				WithProxy(command.Bool(define.FlagUsingSystemProxy)).
				WithEnv(command.StringSlice(define.FlagEnvs)...).
				WithMount(command.StringSlice(define.FlagMount)...).
				WithContainerDiskSpec(containerDiskSpec).
				WithPodmanProxyAPIFile(command.String(define.FlagPodmanProxyAPIFile)).
				WithManageAPIFile(command.String(define.FlagManageAPIFile)).
				WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
				WithRawDiskSpecs(rawDiskSpecs...)

			if u := command.String(define.FlagReportEvents); u != "" {
				cfg.WithEventReporter(u)
			}

			switch cfg.RunMode {
			case revm.ModeAttach:
				return revm.Attach(ctx, cfg)
			case revm.ModeRootfs, revm.ModeContainer:
				vm, err := revm.Build(ctx, cfg)
				if err != nil {
					return err
				}
				defer vm.Release()
				return vm.Run(ctx)
			default:
				return fmt.Errorf("unsupported run mode %q", cfg.RunMode)
			}
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}
