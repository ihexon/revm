//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/eventreporter"
	"linuxvm/pkg/librevm"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

var initCommand = cli.Command{
	Hidden:  true,
	Name:    "cfg",
	Aliases: []string{"init"}, // for compatibility ovm-js ovm init
	Usage:   "generate VM preferences config file",
	Action:  generateOVMCfgAction,
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
		&cli.Uint64Flag{
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

// generateOVMCfgAction generates VM preferences config file from command line and write to vmConfigFilePath
// the baseDir structure is compatible with legacy ovm's workspace to the application can run without any changes
//
// the vm cfg will be load by docker mode and apply those configures
func generateOVMCfgAction(ctx context.Context, cmd *cli.Command) error {
	baseDir, err := filepath.Abs(filepath.Clean(filepath.Join(cmd.String(define.FlagOVMWorkspace), cmd.String(define.FlagOVMName))))
	if err != nil {
		return err
	}

	cfg := librevm.Config{
		RunMode: librevm.ModeCfgGen,
		Disks: map[string]string{
			filepath.Join(baseDir, "data", "source.ext4"): "44f7d1c0-122c-4402-a20e-c1166cbbad6d",
		},
		LogTo:                   filepath.Join(baseDir, "logs", "ovm.log"),
		ExportSSHKeyPrivateFile: filepath.Join(baseDir, "data", "sshkey"),
		ExportSSHKeyPublicFile:  filepath.Join(baseDir, "data", "sshkey.pub"),
		ContainerDisk:           filepath.Join(baseDir, "data", "data.img"),
		PodmanProxyAPIFile:      filepath.Join(baseDir, "socks", "podman-api.sock"),
		ManageAPIFile:           filepath.Join(baseDir, "socks", "ovm_restapi.socks"), // typo, but do not change the name of ovm_restapi.socks
		ContainerDiskVersion:    cmd.String(define.FlagOVMContainerDiskVersion),
		Mounts:                  cmd.StringSlice(define.FlagOVMVolume),
		CPUs:                    cmd.Int(define.FlagCPUS),
		MemoryMB:                cmd.Uint64(define.FlagMemoryInMB),
	}

	if u := cmd.String(define.FlagOVMReportURL); u != "" {
		cfg.WithEventReporter(eventreporter.NewLegacyReporter(u, librevm.ModeCfgGen))
	}

	return librevm.GenerateVMConfig(ctx, &cfg, vmConfigFilePath)
}
