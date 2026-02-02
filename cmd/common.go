package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"os"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func showVersionAndOSInfo() {
	var version strings.Builder
	if define.Version != "" {
		version.WriteString(define.Version)
	} else {
		version.WriteString("unknown")
	}

	version.WriteString("-")

	if define.CommitID != "" {
		version.WriteString(define.CommitID)
	} else {
		version.WriteString(" (unknown)")
	}

	logrus.Infof("%s version: %s", os.Args[0], version.String())
}

func setMaxMemory() uint64 {
	mb, err := system.GetMaxMemoryInMB()
	if err != nil {
		logrus.Warnf("failed to get max memory: %v", err)
		return 512
	}

	return mb
}

func GetVMM(vmc *vmconfig.VMConfig) (interfaces.VMMProvider, error) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return libkrun.NewLibkrunVM(vmc), nil
	}
	return nil, fmt.Errorf("not support this platform")
}

func ConfigureVM(ctx context.Context, command *cli.Command, runMode define.RunMode) (*vmconfig.VMConfig, error) {
	var (
		err              error
		logLevel         = command.String(define.FlagLogLevel)
		saveLogTo        = command.String(define.FlagSaveLogTo)
		workspacePath    = command.String(define.FlagWorkspace)
		cpus             = command.Int8(define.FlagCPUS)
		memoryInMB       = command.Uint64(define.FlagMemoryInMB)
		rawDisks         = command.StringSlice(define.FlagRawDisk)
		rootfsPath       = command.String(define.FlagRootfs)
		usingSystemProxy = command.Bool(define.FlagUsingSystemProxy)

		runBin        = command.Args().First()
		runBinArgs    = command.Args().Tail()
		runBinWorkdir = command.String(define.FlagWorkDir)
		runBinEnvs    = command.StringSlice(define.FlagEnvs)
	)

	vmc := vmconfig.NewVMConfig(runMode)
	logrus.Infof("set run mode: %q", vmc.RunMode)

	if err = vmc.SetLogLevel(logLevel, saveLogTo); err != nil {
		return nil, err
	}
	logrus.Infof("set log level: %s", logLevel)

	if err = vmc.SetupWorkspace(workspacePath); err != nil {
		return nil, err
	}
	logrus.Infof("workspace configure done, path: %q", workspacePath)

	if err = vmc.WithResources(memoryInMB, cpus); err != nil {
		return nil, err
	}
	logrus.Infof("set memory: %dMB, cpus: %d", memoryInMB, cpus)

	if err = vmc.WithGivenRAWDisk(ctx, rawDisks); err != nil {
		return nil, err
	}
	logrus.Infof("given raw disks: %v", rawDisks)

	logrus.Infof("user provided mounts: %v", command.StringSlice(define.FlagMount))
	err = vmc.WithMounts(command.StringSlice(define.FlagMount))
	if err != nil {
		return nil, err
	}

	if err = vmc.SetupIgnitionServerCfg(); err != nil {
		return nil, err
	}
	logrus.Infof("ignition configure done")

	switch vmc.RunMode {
	case define.RootFsMode.String():
		if rootfsPath == "" {
			if err = vmc.WithBuiltInAlpineRootfs(ctx); err != nil {
				return nil, err
			}
			logrus.Infof("user not provided rootfs, use built-in rootfs: %q", vmc.RootFS)
		} else {
			if err = vmc.WithRootfs(ctx, rootfsPath); err != nil {
				return nil, err
			}
			logrus.Infof("user provided rootfs path: %q", rootfsPath)
		}

		if err = vmc.SetupCmdLine(runBinWorkdir, runBin, runBinArgs, runBinEnvs, usingSystemProxy); err != nil {
			return nil, fmt.Errorf("setup cmdline failed: %w", err)
		}
		logrus.Infof("run %q with args %q in %q, using proxy: %t", runBin, runBinArgs, runBinWorkdir, usingSystemProxy)

	case define.ContainerMode.String():
		err = vmc.WithBuiltInAlpineRootfs(ctx)
		if err != nil {
			return nil, err
		}

		if err = vmc.BindUserHomeDir(ctx); err != nil {
			return nil, err
		}

		if err = vmc.AutoAttachContainerStorageRawDisk(ctx); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}

	if err = vmc.SetupGuestAgentCfg(); err != nil {
		return nil, err
	}

	return vmc, nil
}
