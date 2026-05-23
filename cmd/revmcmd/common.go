//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

type LoggingOptions struct {
	Level string
	To    string
}

type PortUpdates struct {
	Exports   []define.PortForward
	Unexports []define.PortForward
}

type RunOptions struct {
	Logging       LoggingOptions
	SessionID     string
	CPUs          int
	MemoryMB      uint64
	Network       string
	UseProxy      bool
	Rootfs        string
	WorkDir       string
	Envs          []string
	Mounts        []string
	RawDisks      []revm.RawDiskSpec
	ManageAPIFile string
	SSHKeyFile    string
	ReportURL     string
	Command       []string
}

type DockerdOptions struct {
	Logging            LoggingOptions
	SessionID          string
	CPUs               int
	MemoryMB           uint64
	UseProxy           bool
	Envs               []string
	Mounts             []string
	RawDisks           []revm.RawDiskSpec
	ContainerDisk      *revm.ContainerDiskSpec
	PodmanProxyAPIFile string
	ManageAPIFile      string
	SSHKeyFile         string
	ReportURL          string
}

type AttachOptions struct {
	Logging   LoggingOptions
	SessionID string
	PTY       bool
	Command   []string
}

type CtlOptions struct {
	Logging      LoggingOptions
	SessionID    string
	ListPort     bool
	PortUpdates  PortUpdates
	ExportRootfs string
	ImportRootfs string
	Command      []string
}

func NewApp(name string) *cli.Command {
	return &cli.Command{
		Name:                      name,
		Usage:                     "run and control revm Linux microVM sessions",
		DisableSliceFlagSeparator: true,
		Commands: []*cli.Command{
			newRunCommand(),
			newDockerdCommand(),
			newAttachCommand(),
			newCtlCommand(),
		},
	}
}

func newRunCommand() *cli.Command {
	return &cli.Command{
		Name:                      "run",
		Usage:                     "boot a Linux VM with a custom rootfs",
		UsageText:                 "run [flags] <command> [args...]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: define.FlagRootfs, Usage: "path to a rootfs directory to use as the VM root filesystem; must contain /bin/sh; takes priority over the built-in rootfs"},
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]); auto-created if the file does not exist; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagWorkDir, Usage: "working directory for command execution inside the guest; the guest-agent chdirs to this path before running the command", Value: "/"},
			&cli.StringFlag{Name: define.FlagVNetworkType, Usage: "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception", Value: string(define.GVISOR)},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock"},
			&cli.StringFlag{Name: define.FlagExportSSHKeyPrivateFile, Usage: "file path to symlink the generated SSH key to"},
			&cli.StringFlag{Name: define.FlagReportEvents, Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)"},
			sessionFlag(),
			logLevelFlag(),
			logToFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, func() (*revm.Config, error) {
				opts, err := ParseRunOptions(command)
				if err != nil {
					return nil, err
				}
				return NewRunConfig(opts), nil
			})
		},
	}
}

func newDockerdCommand() *cli.Command {
	return &cli.Command{
		Name:                      "dockerd",
		Usage:                     "start a Linux VM with the built-in container runtime",
		UsageText:                 "dockerd [flags]",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.Int8Flag{Name: define.FlagCPUS, Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1"},
			&cli.Uint64Flag{Name: define.FlagMemoryInMB, Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512"},
			&cli.StringSliceFlag{Name: define.FlagEnvs, Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagRawDisk, Usage: "attach an ext4 raw disk image to the VM (format: <path>[,uuid=<uuid>][,version=<string>][,mnt=<guest-path>]); auto-created if the file does not exist; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagMount, Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times"},
			&cli.BoolFlag{Name: define.FlagUsingSystemProxy, Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal"},
			&cli.StringFlag{Name: define.FlagContainerDisk, Usage: "persistent ext4 raw disk image for container storage (format: <path>[,version=<string>]); auto-created if missing; if the stored version xattr is missing or mismatched, the disk is recreated; defaults to a workspace-local disk with the built-in container disk version when unset"},
			&cli.StringFlag{Name: define.FlagPodmanProxyAPIFile, Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock"},
			&cli.StringFlag{Name: define.FlagManageAPIFile, Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock"},
			&cli.StringFlag{Name: define.FlagExportSSHKeyPrivateFile, Usage: "file path to symlink the generated SSH key to"},
			&cli.StringFlag{Name: define.FlagReportEvents, Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)"},
			sessionFlag(),
			logLevelFlag(),
			logToFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, func() (*revm.Config, error) {
				opts, err := ParseDockerdOptions(command)
				if err != nil {
					return nil, err
				}
				return NewDockerdConfig(opts), nil
			})
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
			&cli.BoolFlag{Name: define.FlagPTY, Usage: "allocate a pseudo-terminal and launch an interactive shell"},
			sessionFlag(),
			logLevelFlag(),
			logToFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runLogged(ctx, command, func() (*revm.Config, error) {
				opts := ParseAttachOptions(command)
				return NewAttachConfig(opts), nil
			})
		},
	}
}

func newCtlCommand() *cli.Command {
	return &cli.Command{
		Name:                      "ctl",
		Usage:                     "control an existing VM session",
		UsageText:                 "ctl --id <session-id> --list-port\n   ctl --id <session-id> [--port-export spec | --port-unexport spec]\n   ctl --id <session-id> --export-rootfs <path.tar.zst>\n   ctl --id <session-id> --import-rootfs <path.tar.zst>",
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: define.FlagListPort, Usage: "list current gvproxy port mappings for the running VM, including internal SSH forwarding"},
			&cli.StringSliceFlag{Name: define.FlagPortExport, Usage: "expose a guest TCP port on the host (format: [tcp:]<host-port>:<guest-port> or [tcp:]<host-ip>:<host-port>:<guest-port>); updates the running VM and exits; can be specified multiple times"},
			&cli.StringSliceFlag{Name: define.FlagPortUnexport, Usage: "stop exposing a host TCP port for a running VM (format: [tcp:]<host-port> or [tcp:]<host-ip>:<host-port>); can be specified multiple times"},
			&cli.StringFlag{Name: define.FlagExportRootfs, Usage: "export the session rootfs directory to a host tar.zst file"},
			&cli.StringFlag{Name: define.FlagImportRootfs, Usage: "import a host tar.zst file into the session rootfs directory selected by --id"},
			sessionFlag(),
			logLevelFlag(),
			logToFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runCtl(ctx, command)
		},
	}
}

func runLogged(ctx context.Context, command *cli.Command, buildConfig func() (*revm.Config, error)) (retErr error) {
	logFile, err := revm.StartCommandLogging(*newPreflightConfig(command))
	if err != nil {
		return err
	}

	preflight := true
	defer func() {
		if !preflight {
			return
		}
		if retErr != nil {
			logrus.Error(retErr)
		}
		revm.StopCommandLogging(logFile)
	}()

	cfg, err := buildConfig()
	if err != nil {
		return err
	}

	revm.StopCommandLogging(logFile)
	preflight = false
	return revm.Run(ctx, cfg)
}

func runCtl(ctx context.Context, command *cli.Command) (retErr error) {
	logFile, err := revm.StartCommandLogging(*newPreflightConfig(command))
	if err != nil {
		return err
	}

	preflight := true
	defer func() {
		if !preflight {
			return
		}
		if retErr != nil {
			logrus.Error(retErr)
		}
		revm.StopCommandLogging(logFile)
	}()

	opts, err := ParseCtlOptions(command)
	if err != nil {
		return err
	}
	cfg := NewCtlConfig(opts)

	if opts.ListPort {
		ports, err := revm.ListPorts(ctx, cfg)
		if err != nil {
			return err
		}
		return writePortMappings(command.Writer, ports)
	}

	revm.StopCommandLogging(logFile)
	preflight = false
	return revm.Run(ctx, cfg)
}

func newPreflightConfig(command *cli.Command) *revm.Config {
	logging := ParseLoggingOptions(command)
	return revm.DefaultConfig().
		WithLogging(logging.Level, logging.To).
		WithSessionID(command.String(define.FlagSessionID))
}

func ParseLoggingOptions(command *cli.Command) LoggingOptions {
	return LoggingOptions{
		Level: command.String(define.FlagLogLevel),
		To:    command.String(define.FlagLogTo),
	}
}

func ParseRunOptions(command *cli.Command) (RunOptions, error) {
	rawDisks, err := ParseRawDisks(command)
	if err != nil {
		return RunOptions{}, err
	}

	return RunOptions{
		Logging:       ParseLoggingOptions(command),
		SessionID:     command.String(define.FlagSessionID),
		CPUs:          int(command.Int8(define.FlagCPUS)),
		MemoryMB:      command.Uint64(define.FlagMemoryInMB),
		Network:       command.String(define.FlagVNetworkType),
		UseProxy:      command.Bool(define.FlagUsingSystemProxy),
		Rootfs:        command.String(define.FlagRootfs),
		WorkDir:       command.String(define.FlagWorkDir),
		Envs:          command.StringSlice(define.FlagEnvs),
		Mounts:        command.StringSlice(define.FlagMount),
		RawDisks:      rawDisks,
		ManageAPIFile: command.String(define.FlagManageAPIFile),
		SSHKeyFile:    command.String(define.FlagExportSSHKeyPrivateFile),
		ReportURL:     command.String(define.FlagReportEvents),
		Command:       command.Args().Slice(),
	}, nil
}

func ParseDockerdOptions(command *cli.Command) (DockerdOptions, error) {
	rawDisks, err := ParseRawDisks(command)
	if err != nil {
		return DockerdOptions{}, err
	}

	containerDisk, err := ParseContainerDisk(command)
	if err != nil {
		return DockerdOptions{}, err
	}

	return DockerdOptions{
		Logging:            ParseLoggingOptions(command),
		SessionID:          command.String(define.FlagSessionID),
		CPUs:               int(command.Int8(define.FlagCPUS)),
		MemoryMB:           command.Uint64(define.FlagMemoryInMB),
		UseProxy:           command.Bool(define.FlagUsingSystemProxy),
		Envs:               command.StringSlice(define.FlagEnvs),
		Mounts:             command.StringSlice(define.FlagMount),
		RawDisks:           rawDisks,
		ContainerDisk:      containerDisk,
		PodmanProxyAPIFile: command.String(define.FlagPodmanProxyAPIFile),
		ManageAPIFile:      command.String(define.FlagManageAPIFile),
		SSHKeyFile:         command.String(define.FlagExportSSHKeyPrivateFile),
		ReportURL:          command.String(define.FlagReportEvents),
	}, nil
}

func ParseAttachOptions(command *cli.Command) AttachOptions {
	return AttachOptions{
		Logging:   ParseLoggingOptions(command),
		SessionID: command.String(define.FlagSessionID),
		PTY:       command.Bool(define.FlagPTY),
		Command:   command.Args().Slice(),
	}
}

func ParseCtlOptions(command *cli.Command) (CtlOptions, error) {
	portUpdates, err := ParsePortUpdates(command)
	if err != nil {
		return CtlOptions{}, err
	}

	opts := CtlOptions{
		Logging:      ParseLoggingOptions(command),
		SessionID:    command.String(define.FlagSessionID),
		ListPort:     command.Bool(define.FlagListPort),
		PortUpdates:  portUpdates,
		ExportRootfs: command.String(define.FlagExportRootfs),
		ImportRootfs: command.String(define.FlagImportRootfs),
		Command:      command.Args().Slice(),
	}
	if err := validateCtlOptions(opts); err != nil {
		return CtlOptions{}, err
	}
	return opts, nil
}

func ParseRawDisks(command *cli.Command) ([]revm.RawDiskSpec, error) {
	return revm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
}

func ParseContainerDisk(command *cli.Command) (*revm.ContainerDiskSpec, error) {
	value := command.String(define.FlagContainerDisk)
	if value == "" {
		return nil, nil
	}

	spec, err := revm.ParseContainerDiskSpec(value)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

func ParsePortUpdates(command *cli.Command) (PortUpdates, error) {
	portExports, err := revm.ParsePortExportSpecs(command.StringSlice(define.FlagPortExport))
	if err != nil {
		return PortUpdates{}, err
	}

	portUnexports, err := revm.ParsePortUnexportSpecs(command.StringSlice(define.FlagPortUnexport))
	if err != nil {
		return PortUpdates{}, err
	}

	return PortUpdates{
		Exports:   portExports,
		Unexports: portUnexports,
	}, nil
}

func NewRunConfig(opts RunOptions) *revm.Config {
	return revm.DefaultConfig().
		WithLogging(opts.Logging.Level, opts.Logging.To).
		WithSessionID(opts.SessionID).
		WithMode(revm.ModeRootfs).
		WithCommandLine(opts.Command...).
		WithCPUs(opts.CPUs).
		WithMemory(opts.MemoryMB).
		WithNetwork(opts.Network).
		WithProxy(opts.UseProxy).
		WithRootfs(opts.Rootfs).
		WithWorkDir(opts.WorkDir).
		WithEnv(opts.Envs...).
		WithManageAPIFile(opts.ManageAPIFile).
		WithExportSSHKeyPrivateFile(opts.SSHKeyFile).
		WithMount(opts.Mounts...).
		WithRawDiskSpecs(opts.RawDisks...).
		WithEventReporter(opts.ReportURL)
}

func NewDockerdConfig(opts DockerdOptions) *revm.Config {
	return revm.DefaultConfig().
		WithLogging(opts.Logging.Level, opts.Logging.To).
		WithSessionID(opts.SessionID).
		WithMode(revm.ModeContainer).
		WithCPUs(opts.CPUs).
		WithMemory(opts.MemoryMB).
		WithNetwork(string(define.GVISOR)).
		WithProxy(opts.UseProxy).
		WithEnv(opts.Envs...).
		WithMount(opts.Mounts...).
		WithContainerDiskSpec(opts.ContainerDisk).
		WithPodmanProxyAPIFile(opts.PodmanProxyAPIFile).
		WithManageAPIFile(opts.ManageAPIFile).
		WithExportSSHKeyPrivateFile(opts.SSHKeyFile).
		WithRawDiskSpecs(opts.RawDisks...).
		WithEventReporter(opts.ReportURL)
}

func NewAttachConfig(opts AttachOptions) *revm.Config {
	return revm.DefaultConfig().
		WithLogging(opts.Logging.Level, opts.Logging.To).
		WithSessionID(opts.SessionID).
		WithPTY(opts.PTY).
		WithAttach(opts.Command...)
}

func NewCtlConfig(opts CtlOptions) *revm.Config {
	if opts.ListPort {
		return revm.DefaultConfig().
			WithLogging(opts.Logging.Level, opts.Logging.To).
			WithSessionID(opts.SessionID).
			WithPortList()
	}
	if opts.ExportRootfs != "" {
		return revm.DefaultConfig().
			WithLogging(opts.Logging.Level, opts.Logging.To).
			WithSessionID(opts.SessionID).
			WithRootfsExport(opts.ExportRootfs)
	}
	if opts.ImportRootfs != "" {
		return revm.DefaultConfig().
			WithLogging(opts.Logging.Level, opts.Logging.To).
			WithSessionID(opts.SessionID).
			WithRootfsImport(opts.ImportRootfs)
	}

	return revm.DefaultConfig().
		WithLogging(opts.Logging.Level, opts.Logging.To).
		WithSessionID(opts.SessionID).
		WithControl(opts.PortUpdates.Exports, opts.PortUpdates.Unexports)
}

func (p PortUpdates) HasUpdates() bool {
	return len(p.Exports) > 0 || len(p.Unexports) > 0
}

func validateCtlOptions(opts CtlOptions) error {
	if len(opts.Command) > 0 {
		return fmt.Errorf("ctl does not accept command arguments; use revm attach")
	}
	operationCount := 0
	if opts.ListPort {
		operationCount++
	}
	if opts.PortUpdates.HasUpdates() {
		operationCount++
	}
	if opts.ExportRootfs != "" {
		operationCount++
	}
	if opts.ImportRootfs != "" {
		operationCount++
	}
	if operationCount > 1 {
		return fmt.Errorf("ctl control operations cannot be combined")
	}
	if operationCount == 0 {
		return fmt.Errorf("ctl requires --list-port, --port-export, --port-unexport, --export-rootfs, or --import-rootfs")
	}
	return nil
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

func logToFlag() cli.Flag {
	return &cli.StringFlag{Name: define.FlagLogTo, Usage: "custom log file path on host; defaults to /tmp/<session_id>/logs/revm.log"}
}
