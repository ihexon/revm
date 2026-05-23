//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revmcmd

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/revm"

	"github.com/urfave/cli/v3"
)

type ParsedCommon struct {
	RawDisks      []revm.RawDiskSpec
	PortExports   []define.PortForward
	PortUnexports []define.PortForward
}

func ParseCommon(command *cli.Command) (ParsedCommon, error) {
	rawDiskSpecs, err := revm.ParseRawDiskSpecs(command.StringSlice(define.FlagRawDisk))
	if err != nil {
		return ParsedCommon{}, err
	}
	portExports, err := revm.ParsePortExportSpecs(command.StringSlice(define.FlagPortExport))
	if err != nil {
		return ParsedCommon{}, err
	}
	portUnexports, err := revm.ParsePortUnexportSpecs(command.StringSlice(define.FlagPortUnexport))
	if err != nil {
		return ParsedCommon{}, err
	}
	return ParsedCommon{
		RawDisks:      rawDiskSpecs,
		PortExports:   portExports,
		PortUnexports: portUnexports,
	}, nil
}

func ApplyRunMode(command *cli.Command, cfg *revm.Config, defaultMode revm.RunMode, common ParsedCommon) {
	switch {
	case command.Bool(define.FlagAttachMode) && common.HasPortUpdates():
		cfg.WithControl(common.PortExports, common.PortUnexports)
	case command.Bool(define.FlagAttachMode):
		cfg.WithAttach(command.Args().Slice()...)
	default:
		cfg.WithMode(defaultMode).
			WithCommandLine(command.Args().Slice()...)
	}
}

func (c ParsedCommon) HasPortUpdates() bool {
	return len(c.PortExports) > 0 || len(c.PortUnexports) > 0
}
