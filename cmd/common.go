package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func earlyStage(ctx context.Context, command *cli.Command) (context.Context, error) {
	setLogrus(command)
	return ctx, nil
}

func showVersionAndOSInfo() error {
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

	//osInfo, err := system.GetOSVersion()
	//if err != nil {
	//	return fmt.Errorf("failed to get os version: %w", err)
	//}
	//
	//logrus.Infof("os version: %+v", osInfo)

	return nil
}

func setLogrus(command *cli.Command) {
	logrus.SetLevel(logrus.InfoLevel)
	if command.Bool(define.FlagVerbose) {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	})
	logrus.SetOutput(os.Stderr)
}

func setMaxMemory() uint64 {
	mb, err := system.GetMaxMemoryInMB()
	if err != nil {
		logrus.Warnf("failed to get max memory: %v", err)
		return 512
	}

	return mb
}

func GetVMM(vmc *vmconfig.VMConfig) (vm.Provider, error) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return libkrun.NewLibkrunVM(vmc), nil
	}
	return nil, fmt.Errorf("not support this platform")
}


func vmProviderFactory(ctx context.Context, mode define.RunMode, command *cli.Command) (vm.Provider, error) {
	vmc, err := createBaseVMConfig(command)
	if err != nil {
		return nil, fmt.Errorf("failed to create base vm config: %w", err)
	}
	switch mode {
	case define.ContainerMode:
		if err = generateContainerVMConfig(ctx, vmc, command); err != nil {
			return nil, fmt.Errorf("failed to generate container vm config: %w", err)
		}
	case define.RootFsMode:
		if err = generateRootfsVMConfig(ctx, vmc, command); err != nil {
			return nil, fmt.Errorf("failed to generate rootfs vm config: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid run mode: %s", mode.String())
	}

	vmProvider, err := GetVMM(vmc)
	if err != nil {
		return nil, fmt.Errorf("failed to get vm provider: %w", err)
	}

	return vmProvider, nil
}

func createBaseVMConfig(command *cli.Command) (*vmconfig.VMConfig, error) {
	vmc := vmconfig.NewVMConfig()

	if err := vmc.WithExternalTools(); err != nil {
		return nil, fmt.Errorf("failed to find external tools: %w", err)
	}

	vmc.WithResources(command.Uint64(define.FlagMemory), command.Int8(define.FlagCPUS))

	if command.IsSet(define.FlagRestAPIListenAddr) {
		if err := vmc.WithRESTAPIAddress(command.String(define.FlagRestAPIListenAddr)); err != nil {
			return nil, fmt.Errorf("failed to set rest api listen address: %w", err)
		}
	}

	if command.IsSet(define.FlagUsingSystemProxy) {
		if err := vmc.WithSystemProxy(); err != nil {
			return nil, fmt.Errorf("failed to use system proxy: %w", err)
		}
	}

	if err := vmc.GenerateSSHInfo(); err != nil {
		return nil, err
	}

	return vmc, nil
}

func generateContainerVMConfig(ctx context.Context, vmc *vmconfig.VMConfig, command *cli.Command) error {
	vmc.WithRunMode(define.ContainerMode)

	if err := vmc.WithBuiltInRootfs(); err != nil {
		return fmt.Errorf("failed to use builtin rootfs: %w", err)
	}

	if err := vmc.WithContainerDataDisk(ctx, command.String(define.FlagContainerDataStorage)); err != nil {
		return fmt.Errorf("failed to set container data disk: %w", err)
	}

	if err := vmc.WithPodmanListenAPIInHost(command.String(define.FlagListenUnixFile)); err != nil {
		return fmt.Errorf("failed to set podman listen unix file: %w", err)
	}

	if err := vmc.WithShareUserHomeDir(); err != nil {
		return fmt.Errorf("failed to add user home directory to mounts: %w", err)
	}

	if err := vmc.WithGuestAgentConfigure(); err != nil {
		return fmt.Errorf("failed to configure guest agent: %w", err)
	}

	return nil
}

func generateRootfsVMConfig(ctx context.Context, vmc *vmconfig.VMConfig, command *cli.Command) error {
	vmc.WithRunMode(define.RootFsMode)
	if err := vmc.WithUserProvidedRootFS(command.String(define.FlagRootfs)); err != nil {
		return fmt.Errorf("failed to setup rootfs: %w", err)
	}

	if command.IsSet(define.FlagDiskDisk) {
		if err := vmc.WithBlkDisk(ctx, command.StringSlice(define.FlagDiskDisk), false); err != nil {
			return fmt.Errorf("failed to set user provided data disk: %w", err)
		}
	}

	if command.IsSet(define.FlagMount) {
		if err := vmc.WithUserProvidedMounts(command.StringSlice(define.FlagMount)); err != nil {
			return fmt.Errorf("failed to set user provided mounts: %w", err)
		}
	}

	if err := vmc.WithUserProvidedCmdline(command.Args().First(), command.Args().Tail(), command.StringSlice(define.FlagEnvs)); err != nil {
		return fmt.Errorf("failed to set run command and args: %w", err)
	}

	if err := vmc.WithGuestAgentConfigure(); err != nil {
		return fmt.Errorf("failed to configure guest agent: %w", err)
	}

	return nil
}

// func setupVZMode(_ context.Context, vmc *vmconfig.VMConfig, command *cli.Command) (vm.Provider, error) {
//	vmc.RunMode = define.VZMode.String()
//
//	kernelPath, err := path.GetBuiltKernelPath()
//	if err != nil {
//		return nil, fmt.Errorf("failed to find built kernel path: %w", err)
//	}
//	logrus.Infof("kernel path: %s", kernelPath)
//
//	initramfsPath, err := path.GetBuiltInitramfsPath()
//	if err != nil {
//		return nil, fmt.Errorf("failed to find built initramfs path: %w", err)
//	}
//	logrus.Infof("initramfs path: %s", initramfsPath)
//
//	vmc.WithUncompressedKernel(kernelPath)
//	vmc.WithInitramfs(initramfsPath)
//	vmc.WithKernelCmdline([]string{"console=hvc0"})
//
//	// TODO: add env to u-root init
//	if err := vmc.WithUserProvidedCmdline(command.Args().First(), command.Args().Tail(), command.StringSlice(define.FlagEnvs)); err != nil {
//		return nil, err
//	}
//
//	// TODO: auto mount virtio-fs host shared dir
//	if command.IsSet(define.FlagMount) {
//		if err := vmc.WithUserProvidedMounts(command.StringSlice(define.FlagMount)); err != nil {
//			return nil, fmt.Errorf("failed to set user provided mounts: %w", err)
//		}
//	}
//
//	return vm.Get(vmc), nil
//}
