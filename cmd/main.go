//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vm"
	"linuxvm/pkg/vmconfig"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "run a linux shell in 1 second",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "run a linux shell in 1 second",
		Before:                    earlyStage,
		DisableSliceFlagSeparator: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: define.FlagRestAPIListenAddr,
				Usage: "listen for REST API requests on the given address, support http or unix socket address," +
					" e.g. http://127.0.0.1:8080 or unix:///tmp/restapi.sock",
			},
			&cli.BoolFlag{
				Name:   define.FlagVerbose,
				Hidden: true,
				Value:  false,
			},
		},
	}

	app.Commands = []*cli.Command{
		&AttachConsole,
		&startRootfs,
		&startDocker,
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, os.Interrupt)
	if err := app.Run(ctx, os.Args); err != nil {
		logrus.Fatal(err)
	}
}

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

func createVMMProvider(ctx context.Context, command *cli.Command) (vm.Provider, error) {
	vmc := makeVMCfg(command)

	switch command.Name {
	case define.FlagRootfsMode:
		vmc.RunMode = define.RunUserRootfsMode
	case define.FlagDockerMode:
		vmc.RunMode = define.RunDockerEngineMode
	case define.FlagKernelMode:
		vmc.RunMode = define.RunKernelBootMode
	}

	if command.Name == define.FlagDockerMode {
		if err := setDockerModeParameters(vmc, command); err != nil {
			return nil, fmt.Errorf("failed to set docker mode parameters: %w", err)
		}
	}

	// BUG: flock does not work on macOS after the system sleeps and wakes up
	_, err := vmc.Lock()
	if err != nil {
		return nil, err
	}

	if err = vmc.CreateRawDiskWhenNeeded(ctx); err != nil {
		return nil, fmt.Errorf("failed setup raw disk: %w", err)
	}

	if err = vmc.GenerateSSHInfo(); err != nil {
		return nil, err
	}

	if command.Bool(define.FlagUsingSystemProxy) {
		if err = vmc.TryGetSystemProxyAndSetToCmdline(); err != nil {
			return nil, err
		}
	}

	return vm.Get(vmc), nil
}

func makeVMCfg(command *cli.Command) *vmconfig.VMConfig {
	var dataDisks []*define.DataDisk
	for _, disk := range command.StringSlice(define.FlagDiskDisk) {
		dataDisks = append(dataDisks, &define.DataDisk{
			Path: disk,
		})
	}

	prefix := filepath.Join(os.TempDir(), system.GenerateRandomID())
	logrus.Debugf("runtime temp directory: %q", prefix)

	// a stage struct to hold the state of the vm service status
	stage := define.Stage{
		GVProxyChan:   make(chan struct{}, 1),
		IgnServerChan: make(chan struct{}, 1),
	}

	vmc := vmconfig.VMConfig{
		MemoryInMB:          command.Uint64(define.FlagMemory),
		Cpus:                command.Int8(define.FlagCPUS),
		RootFS:              command.String(define.FlagRootfs),
		DataDisk:            dataDisks,
		Mounts:              filesystem.CmdLineMountToMounts(command.StringSlice("mount")),
		GVproxyEndpoint:     fmt.Sprintf("unix://%s/%s", prefix, define.GvProxyControlEndPoint),
		NetworkStackBackend: fmt.Sprintf("unixgram://%s/%s", prefix, define.GvProxyNetworkEndpoint),
		SSHInfo: define.SSHInfo{
			HostSSHKeyPairFile: filepath.Join(prefix, define.SSHKeyPair),
		},

		Cmdline: define.Cmdline{
			Bootstrap:     system.GetGuestLinuxUtilsBinPath(define.BoostrapFileName),
			BootstrapArgs: []string{},
			Workspace:     define.DefaultWorkDir,
			TargetBin:     command.Args().First(),
			TargetBinArgs: command.Args().Tail(),
			Env:           append(command.StringSlice("envs"), define.DefaultPATHInBootstrap),
		},
		RestAPIAddress:     command.String(define.FlagRestAPIListenAddr),
		IgnProvisionerAddr: fmt.Sprintf("unix://%s/%s", prefix, define.IgnServerSocketName),
		Stage:              stage,
	}

	return &vmc
}
