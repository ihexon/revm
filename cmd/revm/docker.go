package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var startDocker = cli.Command{
	Name:                      define.FlagDockerMode,
	Aliases:                   []string{"start", "run"}, // for compatibility
	Usage:                     "start a Linux VM with the built-in container runtime",
	UsageText:                 define.FlagDockerMode + " [flags]",
	Description:               "boot a Linux microVM using libkrun with the built-in rootfs and podman container runtime; exposes a Podman-compatible API socket on the host",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1",
		},
		&cli.StringFlag{
			Name:        define.FlagOVMWorkspace,
			Usage:       "not use any more, retained for compatibility",
			HideDefault: true,
		},
		&cli.Int8Flag{
			Name:        define.FlagOVMPPID,
			Usage:       "not use any more, retained for compatibility",
			HideDefault: true,
		},
		&cli.StringFlag{
			Name:        define.FlagOVMName,
			Usage:       "not use any more, retained for compatibility",
			HideDefault: true,
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
			Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid]); auto-created if the file does not exist; UUID is auto-generated when omitted; mounted at /mnt/<UUID> inside the guest; can be specified multiple times",
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
			Name:  define.FlagReportURL,
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
			Usage: "session name; used to derive the workspace directory (/tmp/<session_id>); defaults to a random string; sessions with the same name are mutually exclusive via flock",
		},
		&cli.StringFlag{
			Name:  define.FlagContainerDisk,
			Usage: "path to a persistent ext4 raw disk image for container storage; auto-created if the file does not exist; defaults to a workspace-local disk if unset",
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
			Name:  define.FlagSSHKeyDir,
			Usage: "directory to symlink the generated SSH key pair (key and key.pub) into; keys are always created inside the session directory",
		},
	},
	Action: dockerLifeCycle,
}

func dockerLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

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

	if u := command.String(define.FlagReportURL); u != "" {
		cfg.WithLegacyEventReport(u)
	}
	if l := command.String(define.FlagLogTo); l != "" {
		cfg.WithLogTo(l)
	}

	// Apply init vmconfig preferences if present.
	if initCfg, err := librevm.LoadFile(vmConfigFilePath); err == nil {
		logrus.Infof("apply vmconfig prefer from: %q", vmConfigFilePath)
		cfg.MergeFrom(initCfg)
		_ = os.Remove(vmConfigFilePath)
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}
	defer vm.Close()

	return vm.Run(ctx)
}
