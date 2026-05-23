//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"

	"github.com/sirupsen/logrus"
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

type BootConfigFunc func(command *cli.Command, cfg *revm.Config) (*revm.Config, error)

type CommandSpec struct {
	DefaultMode   revm.RunMode
	ConfigureBoot BootConfigFunc
}

func Run(ctx context.Context, command *cli.Command, spec CommandSpec) (retErr error) {
	base := newSessionConfig(command)
	logFile, err := revm.StartCommandLogging(*base)
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

	cfg, err := BuildConfig(command, spec, base)
	if err != nil {
		return err
	}

	revm.StopCommandLogging(logFile)
	preflight = false
	return revm.Run(ctx, cfg)
}

func BuildConfig(command *cli.Command, spec CommandSpec, base *revm.Config) (*revm.Config, error) {
	plan, err := ResolveRunPlan(command, spec.DefaultMode)
	if err != nil {
		return nil, err
	}

	switch plan.Mode {
	case revm.ModeControl:
		return base.WithControl(plan.PortUpdates.Exports, plan.PortUpdates.Unexports), nil
	case revm.ModeAttach:
		return base.
			WithPTY(command.Bool(define.FlagPTY)).
			WithAttach(command.Args().Slice()...), nil
	default:
		if spec.ConfigureBoot == nil {
			return nil, fmt.Errorf("boot config function must not be nil")
		}
		return spec.ConfigureBoot(command, base.
			WithPTY(command.Bool(define.FlagPTY)).
			WithMode(plan.Mode).
			WithCommandLine(command.Args().Slice()...))
	}
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

func newSessionConfig(command *cli.Command) *revm.Config {
	return revm.DefaultConfig().
		WithSessionID(command.String(define.FlagSessionID)).
		WithLogging(command.String(define.FlagLogLevel), command.String(define.FlagLogTo))
}
