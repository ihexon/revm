package main

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

var initCmd = cli.Command{
	Name:   define.FlagOVMInit,
	Action: initAction,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name: define.FlagOVMCPUS,
		},
		&cli.Uint64Flag{
			Name: define.FlagOVMMemoryInMB,
		},
		&cli.StringFlag{
			Name: define.FlagOVMBoot,
		},
		&cli.StringFlag{
			Name: define.FlagOVMBootVersion,
		},
		&cli.StringFlag{
			Name: define.FlagOVMContainerDiskVersion,
		},
		&cli.StringFlag{
			Name: define.FlagOVMReportURL,
		},
		&cli.StringFlag{
			Name: define.FlagOVMWorkspace,
		},
		&cli.IntFlag{
			Name: define.FlagOVMPPID,
		},
		&cli.StringSliceFlag{
			Name: define.FlagOVMVolume,
		},
		&cli.StringFlag{
			Name: define.FlagOVMName,
		},
		&cli.StringFlag{
			Name:  define.FlagOVMLogLevel,
			Value: "info",
		},
	},
}

func initAction(ctx context.Context, command *cli.Command) error {
	ovmWorkspace := command.String(define.FlagOVMWorkspace)
	if ovmWorkspace == "" {
		return fmt.Errorf("ovm workspace is required")
	}

	cfg := librevm.Config{
		SessionID:            command.String(define.FlagOVMName),
		ContainerDiskVersion: command.String(define.FlagOVMContainerDiskVersion),
		Mounts:               command.StringSlice(define.FlagOVMVolume),
	}

	return WriteJSONFile(&cfg, filepath.Join(ovmWorkspace, "parameters_from_init.json"))
}

func WriteJSONFile(cfg *librevm.Config, path string) error {
	if path == "" {
		return fmt.Errorf("config file path must not be empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config json: %w", err)
	}

	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write config json file %q: %w", path, err)
	}
	return nil
}
