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
	// Setup logging first (not part of VMConfig)
	logLevel := command.String(define.FlagLogLevel)

	// Extract command-line parameters
	workspacePath := command.String(define.FlagWorkspace)
	cpus := command.Int8(define.FlagCPUS)
	memoryInMB := command.Uint64(define.FlagMemoryInMB)
	rawDisks := command.StringSlice(define.FlagRawDisk)
	mountDirs := command.StringSlice(define.FlagMount)
	networkType := command.String(define.FlagVNetworkType)
	usingSystemProxy := command.Bool(define.FlagUsingSystemProxy)

	// Build VMConfig using builder
	builder := vmconfig.NewVMConfigBuilder(runMode).
		SetWorkspace(workspacePath).
		SetResources(cpus, memoryInMB).
		SetNetworkMode(define.String2NetworkMode(networkType)).
		SetUsingSystemProxy(usingSystemProxy).
		SetRawDisks(rawDisks).
		SetMounts(mountDirs).
		SetLogLevel(logLevel)

	if runMode == define.RootFsMode {
		rootfsPath := command.String(define.FlagRootfs)
		if rootfsPath != "" {
			builder.SetRootfs(rootfsPath)
		} else {
			builder.WithBuiltInRootfs()
		}
		builder.SetCmdline(
			command.String(define.FlagWorkDir),
			command.Args().First(),
			command.Args().Tail(),
			command.StringSlice(define.FlagEnvs),
		)
	}

	vmc, err := builder.Build(ctx)
	if err != nil {
		return nil, err
	}

	return (*vmconfig.VMConfig)(vmc), nil
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
