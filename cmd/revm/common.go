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
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func SetupBasicLogger(level string) error {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}
	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})

	logrus.SetOutput(os.Stderr)
	return nil
}

// RelaunchWithCleanModeBackground launches the standalone helper/clean binary,
// which polls PPID and removes the workspace directory after the parent exits.
func RelaunchWithCleanModeBackground(workspacePath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	cleanBin := filepath.Join(filepath.Dir(execPath), "..", "helper", "clean")

	cleaner := exec.Command(cleanBin)
	cleaner.Stdout = nil
	cleaner.Stderr = nil
	cleaner.Stdin = nil
	cleaner.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cleaner.Env = append(os.Environ(), "WORKSPACE="+workspacePath)

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
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
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

	switch runMode {
	case define.RootFsMode:
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
	case define.ContainerMode:
		rootfsPath := command.String(define.FlagRootfs)
		if rootfsPath != "" {
			builder.SetRootfs(rootfsPath)
		} else {
			builder.WithBuiltInRootfs()
		}
	default:
		return nil, fmt.Errorf("invalid run mode: %s", runMode)
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
