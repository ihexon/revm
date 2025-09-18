package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
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

	if define.CommitID != "" {
		version.WriteString(define.CommitID)
	} else {
		version.WriteString(" (unknown)")
	}

	logrus.Infof("%s version: %s", os.Args[0], version.String())

	osInfo, err := system.GetOSVersion()
	if err != nil {
		return fmt.Errorf("failed to get os version: %w", err)
	}

	logrus.Infof("os version: %+v", osInfo)

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

func VMMProviderFactory(ctx context.Context, mode define.RunMode, command *cli.Command) (vmp vm.Provider, err error) {
	vmc := vmconfig.NewVMConfig()
	vmc.WithResources(command.Uint64(define.FlagMemory), command.Int8(define.FlagCPUS))
	vmc.SetGuestBootstrapRunArgs()

	if command.IsSet(define.FlagRestAPIListenAddr) {
		if err = vmc.WithRESTAPIAddress(command.String(define.FlagRestAPIListenAddr)); err != nil {
			return
		}
	}
	
	if command.IsSet(define.FlagUsingSystemProxy) {
		if err = vmc.WithSystemProxy(); err != nil {
			err = fmt.Errorf("failed to use system proxy: %w", err)
			return
		}
	}

	if err = vmc.GenerateSSHInfo(); err != nil {
		return
	}

	switch mode {
	case define.DockerMode:
		return setupContainerMode(ctx, vmc, command)
	case define.RootFsMode:
		return setupRootfsMode(ctx, vmc, command)
	default:
		err = fmt.Errorf("invalid run mode: %s", mode.String())
		return
	}
}
