package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/event"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/vmbuilder"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// RelaunchWithCleanModeBackground Re-execute in cleanup mode, wait for PPID to become 1, and then perform the cleanup operation.
func RelaunchWithCleanModeBackground(workspacePath string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	cleaner := exec.Command(executable, define.FlagClean, "--workspace", workspacePath)
	cleaner.Stdout = nil
	cleaner.Stderr = nil
	cleaner.Stdin = nil
	cleaner.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return cleaner.Start()
}

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

func GetVMM(mc *define.Machine) (*libkrun.LibkrunVM, error) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return libkrun.NewLibkrunVM(mc), nil
	}
	return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func ConfigureVM(ctx context.Context, command *cli.Command, runMode define.RunMode) (interfaces.VMMProvider, error) {
	event.Emit(event.ConfigureVirtualMachine)

	// Setup logging first (not part of Machine)
	logLevel := command.String(define.FlagLogLevel)

	// Extract command-line parameters
	workspacePath := command.String(define.FlagWorkspace)
	cpus := command.Int8(define.FlagCPUS)
	memoryInMB := command.Uint64(define.FlagMemoryInMB)
	rawDisks := command.StringSlice(define.FlagRawDisk)
	mountDirs := command.StringSlice(define.FlagMount)
	networkType := command.String(define.FlagVNetworkType)
	usingSystemProxy := command.Bool(define.FlagUsingSystemProxy)

	if command.Args().Len() < 1 && runMode == define.RootFsMode {
		return nil, fmt.Errorf("no command specified")
	}

	if memoryInMB < 512 {
		m, err := mem.VirtualMemory()
		if err != nil {
			return nil, err
		}
		memoryInMB = m.Total / 1024 / 1024
	}

	if cpus < 1 {
		cpus = int8(runtime.NumCPU())
	}

	// Build Machine using builder
	builder := vmbuilder.NewVMConfigBuilder(runMode).
		SetWorkspace(workspacePath).
		SetResources(cpus, memoryInMB).
		SetNetworkMode(define.VNetMode(networkType)).
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

	if !command.IsSet(define.FlagWorkspace) {
		if err = RelaunchWithCleanModeBackground(workspacePath); err != nil {
			return nil, err
		}
	}

	vmp, err := GetVMM(vmc)
	if err != nil {
		return nil, err
	}

	return vmp, nil
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
