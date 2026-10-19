//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/revm"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"
)

func NewApp(name string) *cli.Command {
	app := &cli.Command{
		Name:                            name,
		Version:                         versionString(),
		EnableShellCompletion:           true,
		ConfigureShellCompletionCommand: configureCompletion,
		Usage:                           "run and control revm Linux microVM sessions",
		DisableSliceFlagSeparator:       true,
		Commands: []*cli.Command{
			newRunCommand(),
			newPodmanCommand(),
			newAttachCommand(),
			newCtlCommand(),
		},
	}
	app.Commands = append(app.Commands, resourceCommands()...)
	app.Commands = append(app.Commands, sessionCommands()...)
	app.Action = func(ctx context.Context, c *cli.Command) error {
		if c.Args().Len() != 0 {
			return commanderr.Usage(fmt.Errorf("unknown command %q; use revm --help", c.Args().First()))
		}
		return cli.ShowAppHelp(c)
	}
	configureErrors(app)
	return app
}

func configureErrors(c *cli.Command) {
	if c.Action == nil && len(c.Commands) > 0 {
		c.Action = func(ctx context.Context, c *cli.Command) error {
			if c.Args().Len() > 0 {
				return commanderr.Usage(fmt.Errorf("unknown %s command %q", c.Name, c.Args().First()))
			}
			return cli.ShowSubcommandHelp(c)
		}
	}
	c.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	c.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error { return commanderr.Usage(err) }
	for _, child := range c.Commands {
		configureErrors(child)
	}
}

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:                      "run",
		Usage:                     "boot the built-in Alpine Linux VM",
		UsageText:                 "run [flags] <command> [args...]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "rm", Usage: "remove session records and logs after exit; preserve persistent data"},
			&cli.StringFlag{Name: "tty", Value: "auto", Usage: "terminal allocation: auto, always or never"},
			&cli.BoolFlag{Name: "pty", Usage: "compatibility alias for --tty=always"},
			&cli.DurationFlag{Name: "stop-timeout", Value: 10 * time.Second, Usage: "graceful cancellation budget"},
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to min(host CPU count, 4)"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to one quarter of available memory, between 512 and 2048 MB"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach a raw disk (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=relaxed|full|none]); auto-created if missing; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagWorkDir, Usage: "working directory for command execution inside the guest; the guest-agent chdirs to this path before running the command", Value: "/"},
			&cli.StringFlag{Name: define.FlagVNetworkType, Usage: "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception", Value: string(define.GVISOR)},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to the current instance runtime directory"},
			&cli.StringFlag{Name: define.FlagReportEvents, Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)"},
			sessionFlag(),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, revm.ModeRootfs)
		},
	}
}

func newPodmanCommand() *cli.Command {
	return &cli.Command{
		Name:                      "podman",
		Usage:                     "start a Linux VM running the Podman API service",
		UsageText:                 "podman [flags]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.DurationFlag{Name: "stop-timeout", Value: 10 * time.Second, Usage: "graceful cancellation budget"},
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to min(host CPU count, 4)"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to one quarter of available memory, between 512 and 2048 MB"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach a raw disk (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=relaxed|full|none]); auto-created if missing; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagContainerDisk, Usage: "persistent ext4 raw disk image for container storage (format: <path>[,version=<string>]); auto-created if missing; existing disks are inspected and never replaced automatically; defaults to an independent managed volume"},
			&cli.StringFlag{Name: define.FlagPodmanProxyAPIFile, Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to the current instance runtime directory"},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to the current instance runtime directory"},
			&cli.StringFlag{Name: define.FlagReportEvents, Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)"},
			sessionFlag(),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, revm.ModeContainer)
		},
	}
}

func newAttachCommand() *cli.Command {
	return &cli.Command{
		Name:                      "attach",
		Usage:                     "attach to an existing VM session",
		UsageText:                 "attach --id <session-id> [--pty] [-- <command> [args...]]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: define.FlagPTY, Usage: "compatibility alias for --tty=always"},
			&cli.StringFlag{Name: "tty", Value: "auto", Usage: "terminal allocation: auto, always or never"},
			&cli.DurationFlag{Name: "stop-timeout", Value: 10 * time.Second, Usage: "graceful cancellation budget"},
			sessionFlag(),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, revm.ModeAttach)
		},
	}
}

func newCtlCommand() *cli.Command {
	return &cli.Command{
		Name:                      "ctl",
		Usage:                     "control an existing VM session",
		UsageText:                 "ctl --id <session-id> --list-port\n   ctl --id <session-id> [--port-export spec | --port-unexport spec]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "print port mappings as JSON (requires --list-port)"},
			&cli.BoolFlag{Name: define.FlagListPort, Usage: "list current gvproxy port mappings for the running VM"},
			&cli.StringSliceFlag{Name: define.FlagPortExport, Usage: "expose a guest TCP port on the host (format: [tcp:]<host-port>:<guest-port> or [tcp:]<host-ip>:<host-port>:<guest-port>); updates the running VM and exits; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagPortUnexport, Usage: "stop exposing a host TCP port for a running VM (format: [tcp:]<host-port> or [tcp:]<host-ip>:<host-port>); can be specified multiple times"},
			sessionFlag(),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runCtl(ctx, command)
		},
	}
}

func runLogged(ctx context.Context, command *cli.Command, mode revm.RunMode) error {
	cfg, err := parseConfig(command, mode)
	if err != nil {
		return commanderr.Usage(err)
	}
	cfg.Remove = command.Bool("rm")
	cfg.TTY = command.String("tty")
	cfg.StopTimeout = command.Duration("stop-timeout")
	if command.Bool("pty") {
		if command.IsSet("tty") && cfg.TTY != "always" {
			return commanderr.Usage(fmt.Errorf("--pty conflicts with --tty=%s", cfg.TTY))
		}
		cfg.TTY = "always"
	}
	if controller := execution.ControllerFrom(ctx); controller != nil {
		controller.SetTimeout(cfg.StopTimeout)
	}
	return revm.Run(ctx, cfg)
}

func runCtl(ctx context.Context, command *cli.Command) error {
	cfg, err := parseConfig(command, revm.ModeControl)
	if err != nil {
		return commanderr.Usage(err)
	}
	if command.Bool("json") && !cfg.PortList {
		return commanderr.Usage(fmt.Errorf("--json requires --list-port"))
	}
	if cfg.PortList {
		ports, err := revm.ListPorts(ctx, cfg)
		if err != nil {
			return err
		}
		if command.Bool("json") {
			if ports == nil {
				ports = []define.PortMapping{}
			}
			return json.NewEncoder(command.Writer).Encode(ports)
		}
		return writePortMappings(command.Writer, ports)
	}
	return revm.Run(ctx, cfg)
}

// Parse directly into the one runtime input configuration. Validation and
// default resolution happen once at the public Run/ListPorts entry points.
func parseConfig(c *cli.Command, mode revm.RunMode) (*revm.Config, error) {
	cfg := revm.DefaultConfig()
	cfg.RunMode, cfg.SessionID, cfg.LogLevel = mode, c.String(define.FlagSessionID), c.String(define.FlagLogLevel)
	cfg.CPUs, cfg.MemoryMB = int(c.Int8(define.FlagCPUS)), c.Uint64(define.FlagMemoryInMB)
	cfg.Network, cfg.Proxy = c.String(define.FlagVNetworkType), c.Bool(define.FlagUsingSystemProxy)
	cfg.Exec = execution.Request{Argv: c.Args().Slice(), Env: c.StringSlice(define.FlagEnvs), WorkDir: c.String(define.FlagWorkDir)}
	cfg.Mounts = c.StringSlice(define.FlagMount)
	cfg.ManageAPIFile, cfg.PodmanProxyAPIFile = c.String(define.FlagManageAPIFile), c.String(define.FlagPodmanProxyAPIFile)
	cfg.ReportURL = c.String(define.FlagReportEvents)
	cfg.PortList = c.Bool(define.FlagListPort)
	var err error
	if cfg.Disks, err = revm.ParseRawDiskSpecs(c.StringSlice(define.FlagRawDisk)); err != nil {
		return nil, err
	}
	if value := c.String(define.FlagContainerDisk); value != "" {
		disk, err := revm.ParseContainerDiskSpec(value)
		if err != nil {
			return nil, err
		}
		cfg.ContainerDisk = &disk
	}
	if cfg.PortForwards, err = revm.ParsePortExportSpecs(c.StringSlice(define.FlagPortExport)); err != nil {
		return nil, err
	}
	if cfg.PortUnforwards, err = revm.ParsePortUnexportSpecs(c.StringSlice(define.FlagPortUnexport)); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writePortMappings(w io.Writer, ports []define.PortMapping) error {
	if w == nil {
		w = os.Stdout
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Local == ports[j].Local {
			return ports[i].Protocol < ports[j].Protocol
		}
		return ports[i].Local < ports[j].Local
	})

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "PROTOCOL\tHOST\tGUEST"); err != nil {
		return err
	}
	for _, port := range ports {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", port.Protocol, port.Local, port.Remote); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func sessionFlag() cli.Flag {
	return &cli.StringFlag{Name: define.FlagSessionID, Usage: "required session name", Required: true}
}

func logLevelFlag() cli.Flag {
	return &cli.StringFlag{Name: define.FlagLogLevel, Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)", Value: "info"}
}
