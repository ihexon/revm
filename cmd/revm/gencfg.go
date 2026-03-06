//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

var initCommand = cli.Command{
	Hidden:  true,
	Name:    define.SubCommand,
	Aliases: []string{"init"},
	Usage:   "generate VM preferences config file",
	Action:  generateVMCfgAction,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name: define.FlagCPUS,
		},
		&cli.Uint64Flag{
			Name: define.FlagMemoryInMB,
		},
		&cli.StringFlag{
			Name: define.FlagOVMWorkspace,
		},
		&cli.StringFlag{
			Name:  define.FlagOVMBoot,
			Usage: "not use any more, retained for compatibility",
		},
		&cli.StringFlag{
			Name: define.FlagOVMBootVersion,
		},
		&cli.StringFlag{
			Name: define.FlagOVMContainerDiskVersion,
		},
		&cli.StringFlag{
			Name: define.FlagReportURL,
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
			Name:  define.FlagLogLevel,
			Value: "info",
		},
	},
}

const vmConfigFilePath = "/tmp/vmcfg-afd8c036e065.json"

func generateVMCfgAction(ctx context.Context, cmd *cli.Command) error {
	baseDir, err := filepath.Abs(filepath.Clean(filepath.Join(cmd.String(define.FlagOVMWorkspace), cmd.String(define.FlagOVMName))))
	if err != nil {
		return err
	}

	cfg := librevm.Config{
		Disks: map[string]string{
			filepath.Join(baseDir, "data", "source.ext4"): "68fa285b-f392-4e79-a8ea-34419c7fd026",
		},
		SSHKeyDir:            filepath.Join(baseDir, "ssh"),
		ContainerDisk:        filepath.Join(baseDir, "data", "data.img"),
		PodmanProxyAPIFile:   filepath.Join(baseDir, "socks", "podman-api.sock"),
		ManageAPIFile:        filepath.Join(baseDir, "socks", "ovm_restapi.socks"), // typo, but do not change the name of ovm_restapi.socks
		ContainerDiskVersion: cmd.String(define.FlagOVMContainerDiskVersion),
		Mounts:               cmd.StringSlice(define.FlagOVMVolume),
		LegacyEventReportURL: cmd.String(define.FlagReportURL),
		CPUs:                 cmd.Int(define.FlagCPUS),
		MemoryMB:             cmd.Uint64(define.FlagMemoryInMB),
	}

	return librevm.GenerateVMConfig(ctx, &cfg, vmConfigFilePath)
}
