package main

import (
	"context"
	"linuxvm/pkg/define"
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
			Usage: "attach an ext4 raw disk image to the VM; auto-created as a 10 GB ext4 image if the path does not exist; mounted at /mnt/<UUID> inside the guest; can be specified multiple times",
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
			Name:  define.FlagWorkDir,
			Usage: "working directory for command execution inside the guest; the guest-agent chdirs to this path before running the command",
			Value: "/",
		},
		&cli.StringFlag{
			Name:   define.FlagVNetworkType,
			Usage:  "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception",
			Value:  string(define.GVISOR),
			Hidden: false,
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
			Usage: "custom log file path on host; defaults to <workspace>/logs/vm.log when unset",
		},
		&cli.StringFlag{
			Name:  define.FlagSessionID,
			Usage: "session name; used to derive the workspace directory (/tmp/<name>); defaults to a random string; sessions with the same name are mutually exclusive via flock",
		},
		&cli.StringFlag{
			Name:  define.FlagManageAPI,
			Usage: "custom Unix socket path for the host-side VM management API; defaults to <workspace>/socks/vmctl.sock",
		},
	},
	Action: rootfsLifeCycle,
}

func rootfsLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

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

	if u := command.String(define.FlagReportURL); u != "" {
		cfg.WithLegacyEventReport(u)
	}
	if l := command.String(define.FlagLogTo); l != "" {
		cfg.WithLogTo(l)
	}
	if m := command.String(define.FlagManageAPI); m != "" {
		cfg.WithManageAPI(m)
	}

	vm, err := librevm.New(ctx, cfg)
	if err != nil {
		return err
	}

	defer vm.Close()

	return vm.Run(ctx)
}
