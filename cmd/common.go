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
	"path/filepath"
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
		err           error
		logLevel      = command.String(define.FlagLogLevel)
		saveLogTo     = command.String(define.FlagSaveLogTo)
		workspacePath = command.String(define.FlagWorkspace)
		cpus          = command.Int8(define.FlagCPUS)
		memoryInMB    = command.Uint64(define.FlagMemoryInMB)
		rawDisks      = command.StringSlice(define.FlagRawDisk)
	)

	vmc := vmconfig.NewVMConfig(runMode)
	logrus.Infof("set run mode: %q", vmc.RunMode)

	if err = vmc.SetLogLevel(logLevel, saveLogTo);err != nil {
		return nil, err
	}

	if err = vmc.SetupWorkspace(workspacePath); err != nil {
		return nil, err
	}

	if err = vmc.WithResources(memoryInMB, cpus); err != nil {
		return nil, err
	}

	err = vmc.WithGivenRAWDisk(ctx, rawDisks)
	if err != nil {
		return nil, err
	}

	logrus.Infof("setup mounts to %v", command.StringSlice(define.FlagMount))
	err = vmc.WithMounts(command.StringSlice(define.FlagMount))
	if err != nil {
		return nil, err
	}

	logrus.Infof("setup ignition config")
	err = vmc.SetupIgnition()
	if err != nil {
		return nil, err
	}

	usingSystemProxy := false
	if command.Bool(define.FlagUsingSystemProxy) {
		usingSystemProxy = true
	}

	switch vmc.RunMode {
	case define.RootFsMode.String():
		if !command.IsSet(define.FlagRootfs) {
			if err = command.Set(define.FlagRootfs, filepath.Join(vmc.WorkspacePath, "builtin-rootfs")); err != nil {
				return nil, err
			}
		}

		if err = vmc.WithRootfs(ctx, command.String(define.FlagRootfs)); err != nil {
			return nil, err
		}

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

	return vmc, vmc.RunCmdline(command.String(define.FlagWorkDir),
		command.Args().First(),
		command.Args().Tail(),
		command.StringSlice(define.FlagEnvs),
		usingSystemProxy)
}
