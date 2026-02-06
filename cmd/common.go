package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"time"

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

		networkType = command.String(define.FlagNetwork)
	)

	vmc := vmconfig.NewVMConfig(runMode)

	if err = vmc.SetLogLevel(logLevel, saveLogTo); err != nil {
		return nil, err
	}

	// save the cancel Fn to vmc, so we can call CancelFn in vm lifycycle any time
	cancelFn, ok := ctx.Value(define.CancelFnKey).(context.CancelFunc)
	if !ok {
		return nil, fmt.Errorf("cancel function can not set")
	}

	vmc.CancelFn = cancelFn

	if err = vmc.SetupWorkspace(workspacePath); err != nil {
		return nil, err
	}

	if err = vmc.WithResources(memoryInMB, cpus); err != nil {
		return nil, err
	}

	if err = vmc.WithUserProvidedStorageRAWDisk(ctx, rawDisks); err != nil {
		return nil, err
	}

	if err = vmc.WithMounts(command.StringSlice(define.FlagMount)); err != nil {
		return nil, err
	}

	if err = vmc.SetupIgnitionServerCfg(); err != nil {
		return nil, err
	}

	switch vmc.RunMode {
	case define.RootFsMode.String():
		if rootfsPath == "" {
			if err = vmc.WithBuiltInAlpineRootfs(ctx); err != nil {
				return nil, err
			}
		} else {
			if err = vmc.WithRootfs(ctx, rootfsPath); err != nil {
				return nil, err
			}
		}

		if err = vmc.SetupCmdLine(runBinWorkdir, runBin, runBinArgs, runBinEnvs, usingSystemProxy); err != nil {
			return nil, fmt.Errorf("setup cmdline failed: %w", err)
		}

	case define.ContainerMode.String():
		if err = vmc.WithBuiltInAlpineRootfs(ctx); err != nil {
			return nil, err
		}

		if err = vmc.BindUserHomeDir(ctx); err != nil {
			return nil, err
		}

		if err = vmc.AttachOrGenerateContainerStorageRawDisk(ctx); err != nil {
			return nil, err
		}

		if usingSystemProxy {
			if err = vmc.ConfigurePodmanUsingSystemProxy(); err != nil {
				return nil, fmt.Errorf("failed to configure podman using system proxy: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}

	if err = vmc.SetupGuestAgentCfg(); err != nil {
		return nil, err
	}

	if networkType == "tsi" {
		if err = vmc.WithNetworkTSI(); err != nil {
			return nil, err
		}
	}

	logrus.Infof("VM configured: mode=%s, cpus=%d, memory=%dMB", vmc.RunMode, cpus, memoryInMB)
	return vmc, nil
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

func FastRandomStr() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = base62[rng.Intn(62)]
	}
	return string(b)
}
