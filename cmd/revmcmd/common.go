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
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/revm"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

func NewApp(name string) *cli.Command {
	app := &cli.Command{
		Name:                            name,
		Version:                         versionString(),
		ConfigureShellCompletionCommand: configureCompletion,
		Usage:                           "run and control revm Linux microVM sessions",
		DisableSliceFlagSeparator:       true,
		Commands: []*cli.Command{
			newRunCommand(),
			newPodmanCommand(),
			newExecCommand(),
			newCtlCommand(),
			newCompleteCommand(),
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
	configureHelp(app)
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
		UsageText:                 "revm run [options] -- COMMAND [ARG...]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "tty", Value: "auto", Usage: "terminal allocation: auto, always or never"},
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to min(host CPU count, 4)"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to one quarter of available memory, between 512 and 2048 MB"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach a raw disk (format: <path>[,uuid=<uuid>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=relaxed|full|none]); auto-created if missing; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory with the guest (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagWorkDir, Usage: "absolute working directory inside the guest", Value: "/"},
			&cli.StringFlag{Name: define.FlagVNetworkType, Usage: "network mode: gvisor supports port forwarding; tsi uses host socket connections", Value: string(define.GVISOR)},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to the current instance runtime directory"},
			sessionFlag(false),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runVM(ctx, command, define.RootFsMode)
		},
	}
}

func newPodmanCommand() *cli.Command {
	return &cli.Command{
		Name:                      "podman",
		Usage:                     "start a Linux VM running the Podman API service",
		UsageText:                 "revm podman [options] (--volume NAME | --container-disk PATH)",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to min(host CPU count, 4)"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to one quarter of available memory, between 512 and 2048 MB"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach a raw disk (format: <path>[,uuid=<uuid>][,mnt=<guest-path>][,readonly=true][,directio=true][,sync=relaxed|full|none]); auto-created if missing; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory with the guest (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: "volume", Usage: "existing persistent volume name (create with revm volume create NAME)"},
			&cli.StringFlag{Name: define.FlagContainerDisk, Usage: "persistent ext4 raw disk image for container storage (format: <path>); auto-created if missing; existing disks are inspected and never replaced automatically; mutually exclusive with --volume"},
			&cli.StringFlag{Name: define.FlagPodmanProxyAPIFile, Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to the current instance runtime directory"},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to the current instance runtime directory"},
			sessionFlag(false),
			logLevelFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runVM(ctx, command, define.ContainerMode)
		},
	}
}

func newExecCommand() *cli.Command {
	return &cli.Command{
		Name:                      "exec",
		Usage:                     "execute a command in a running VM session",
		UsageText:                 "revm exec --id SESSION [options] [-- COMMAND [ARG...]]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "tty", Value: "auto", Usage: "terminal allocation: auto, always or never"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "guest environment (KEY=VALUE); repeatable"},
			&cli.StringFlag{Name: define.FlagWorkDir, Value: "/", Usage: "absolute guest working directory"},
			sessionFlag(true),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return revm.ExecSession(ctx, command.String(define.FlagSessionID), execOptions(command))
		},
	}
}

func newCtlCommand() *cli.Command {
	return &cli.Command{
		Name:                      "ctl",
		Usage:                     "control an existing VM session",
		UsageText:                 "revm ctl --id SESSION --list-port [--json]\n   revm ctl --id SESSION [--port-export SPEC] [--port-unexport SPEC]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "print port mappings as JSON (requires --list-port)"},
			&cli.BoolFlag{Name: define.FlagListPort, Usage: "list current gvproxy port mappings for the running VM"},
			&cli.StringSliceFlag{Name: define.FlagPortExport, Usage: "expose a guest TCP port on the host (format: [tcp:]<host-port>:<guest-port> or [tcp:]<host-ip>:<host-port>:<guest-port>); updates the running VM and exits; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagPortUnexport, Usage: "stop exposing a host TCP port for a running VM (format: [tcp:]<host-port> or [tcp:]<host-ip>:<host-port>); can be specified multiple times"},
			sessionFlag(true),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runCtl(ctx, command)
		},
	}
}

func runVM(ctx context.Context, command *cli.Command, mode define.RunMode) error {
	cfg, err := parseVMOptions(command, mode)
	if err != nil {
		return commanderr.Usage(err)
	}
	return revm.RunVM(ctx, *cfg)
}

func runCtl(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() != 0 {
		return commanderr.Usage(fmt.Errorf("ctl does not accept a command"))
	}
	exports, err := revm.ParsePortExportSpecs(command.StringSlice(define.FlagPortExport))
	if err != nil {
		return commanderr.Usage(err)
	}
	unexports, err := revm.ParsePortUnexportSpecs(command.StringSlice(define.FlagPortUnexport))
	if err != nil {
		return commanderr.Usage(err)
	}
	list := command.Bool(define.FlagListPort)
	if list == (len(exports)+len(unexports) > 0) {
		return commanderr.Usage(fmt.Errorf("select either --list-port or port updates"))
	}
	if command.Bool("json") && !list {
		return commanderr.Usage(fmt.Errorf("--json requires --list-port"))
	}
	session := command.String(define.FlagSessionID)
	if !list {
		return revm.UpdatePorts(ctx, session, exports, unexports)
	}
	ports, err := revm.ListPorts(ctx, session)
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

func execOptions(c *cli.Command) revm.ExecOptions {
	return revm.ExecOptions{Exec: execution.Request{Argv: c.Args().Slice(), Env: c.StringSlice(define.FlagEnvs), WorkDir: c.String(define.FlagWorkDir)}, TTY: c.String("tty")}
}

func parseVMOptions(c *cli.Command, mode define.RunMode) (*revm.VMOptions, error) {
	cfg := &revm.VMOptions{
		RunMode: mode, SessionID: c.String(define.FlagSessionID), LogLevel: c.String(define.FlagLogLevel),
		CPUs: int(c.Int8(define.FlagCPUS)), MemoryMB: c.Uint64(define.FlagMemoryInMB),
		Network: c.String(define.FlagVNetworkType), Proxy: c.Bool(define.FlagUsingSystemProxy),
		ExecOptions: execOptions(c), VolumeName: c.String("volume"),
		ManageAPIFile: c.String(define.FlagManageAPIFile), PodmanProxyAPIFile: c.String(define.FlagPodmanProxyAPIFile),
	}
	var err error
	if cfg.Mounts, err = filesystem.ParseMounts(c.StringSlice(define.FlagMount)); err != nil {
		return nil, err
	}
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

func sessionFlag(required bool) cli.Flag {
	usage := "session name; retain successful runs too (failed runs are always retained)"
	if required {
		usage = "name of the running session (see revm ls)"
	}
	return &cli.StringFlag{Name: define.FlagSessionID, Usage: usage, Required: required}
}

func logLevelFlag() cli.Flag {
	return &cli.StringFlag{Name: define.FlagLogLevel, Usage: "verbosity of saved logs; read with revm logs SESSION (trace, debug, info, warn, error, fatal, panic)", Value: "info"}
}
