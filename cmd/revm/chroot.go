package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	"linuxvm/pkg/service"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startRootfs = cli.Command{
	Name:        define.FlagChroot,
	Aliases:     []string{"run"},
	Usage:       "boot a Linux VM with a custom rootfs",
	UsageText:   define.FlagChroot + " [flags] <command> [args...]",
	Description: "boot a Linux microVM using libkrun and execute commands inside it, similar to chroot but with full kernel isolation",
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
			Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888); events include: ConfigureVirtualMachine, StartVirtualNetwork, StartIgnitionServer, StartVirtualMachine, GuestNetworkReady, GuestSSHReady, GuestPodmanReady, Exit, Error",
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagWorkspace,
			Usage: "directory for VM runtime state: Unix sockets (podman API, gvproxy ctl, ignition), SSH keys, guest logs, and auto-created disk images; cannot be the home directory",
			Value: fmt.Sprintf("/tmp/.revm-%s", FastRandomStr()),
		},
	},
	Action: rootfsLifeCycle,
}

func rootfsLifeCycle(ctx context.Context, command *cli.Command) error {
	showVersionAndOSInfo()

	event.Setup(command.String(define.FlagReportURL), event.Chroot)

	vmp, err := ConfigureVM(ctx, command, define.RootFsMode)
	if err != nil {
		return fmt.Errorf("configure vm fail: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	svc := service.NewHostServices(vmp)
	g.Go(func() error {
		return svc.ExitVirtualMachineWhenSomethingHappened(ctx)
	})

	g.Go(func() error {
		return svc.StartIgnitionService(ctx)
	})

	g.Go(func() error {
		return svc.StartNetworkStack(ctx)
	})

	g.Go(func() error {
		return svc.StartMachineManagementAPI(ctx)
	})

	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vmp.GetVMConfigure().Readiness.VNetHostReady:
			return svc.StartVirtualMachine(ctx)
		}
	})

	errChan := make(chan error, 1)
	go func() { errChan <- g.Wait() }()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err = <-errChan:
		return err
	}
}
