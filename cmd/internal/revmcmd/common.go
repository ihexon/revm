//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"

	"github.com/urfave/cli/v3"
)

type ParsedCommon struct {
	RawDisks []revm.RawDiskSpec
}

type PortUpdates struct {
	Exports   []define.PortForward
	Unexports []define.PortForward
}

type RunPlan struct {
	Mode        revm.RunMode
	PortUpdates PortUpdates
}

func ResolveRunPlan(command *cli.Command, defaultMode revm.RunMode) (RunPlan, error) {
	ports, err := ParsePortUpdates(command)
	if err != nil {
		return RunPlan{}, err
	}

	return NewRunPlan(command.Bool(define.FlagAttachMode), ports, defaultMode), nil
}

func NewRunPlan(attach bool, ports PortUpdates, defaultMode revm.RunMode) RunPlan {
	switch {
	case ports.HasUpdates():
		return RunPlan{Mode: revm.ModeControl, PortUpdates: ports}
	case attach:
		return RunPlan{Mode: revm.ModeAttach}
	default:
		return RunPlan{Mode: defaultMode}
	}
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

func ParseCommon(command *cli.Command) (ParsedCommon, error) {
	rawDiskSpecs, err := revm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
	if err != nil {
		return ParsedCommon{}, err
	}
	return ParsedCommon{
		RawDisks: rawDiskSpecs,
	}, nil
}

func (p PortUpdates) HasUpdates() bool {
	return len(p.Exports) > 0 || len(p.Unexports) > 0
}

func NewControlConfig(command *cli.Command, ports PortUpdates) *revm.Config {
	return newSessionConfig(command).
		WithControl(ports.Exports, ports.Unexports)
}

func NewAttachConfig(command *cli.Command) *revm.Config {
	return newSessionConfig(command).
		WithPTY(command.Bool(define.FlagPTY)).
		WithAttach(command.Args().Slice()...)
}

func NewBootConfig(command *cli.Command, mode revm.RunMode) *revm.Config {
	return newSessionConfig(command).
		WithPTY(command.Bool(define.FlagPTY)).
		WithMode(mode).
		WithCommandLine(command.Args().Slice()...)
}

func newSessionConfig(command *cli.Command) *revm.Config {
	return revm.DefaultConfig().
		WithSessionID(command.String(define.FlagSessionID)).
		WithLogging(command.String(define.FlagLogLevel), command.String(define.FlagLogTo))
}
