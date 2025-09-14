package main

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/network"
	"linuxvm/pkg/server"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var startDocker = cli.Command{
	Name:        define.FlagDockerMode,
	Aliases:     []string{"docker", "podman", "podman-mode", "container-mode", "container"},
	Usage:       "run in Docker-compatible mode",
	UsageText:   define.FlagDockerMode + " [OPTIONS] [command]",
	Description: "In Docker compatibility mode, the built-in Docker engine is used and a unix socks file is listened to as the API entry point used by the docker cli.",
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "given how many cpu cores",
			Value: int8(system.GetCPUCores()),
		},
		&cli.Uint64Flag{
			Name:    define.FlagMemory,
			Aliases: []string{"m"},
			Usage:   "given how many memory in MB",
			Value:   setMaxMemory(),
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "use system proxy, set environment http(s)_proxy to docker engine",
			Value: false,
		},
		&cli.StringFlag{
			Name:    define.FlagRootfs,
			Aliases: []string{"d", "podman-rootfs"},
			Usage:   "path to another podman rootfs directory (must have podman pre-installed)",
		},
		&cli.StringFlag{
			Name:    define.FlagListenUnixFile,
			Aliases: []string{"l"},
			Usage:   "listen for Docker API requests on a Unix socket, forwarding them to the guest's Docker engine",
			Value:   define.DefaultPodmanAPIUnixSocksInHost,
		},
		&cli.StringFlag{
			Name:     define.FlagContainerDataStorage,
			Aliases:  []string{"data", "s", "save"},
			Usage:    "An raw data disk that save all container data",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "mount host dir to guest dir",
		},
	},
	Action: dockerModeLifeCycle,
}

func dockerModeLifeCycle(ctx context.Context, command *cli.Command) error {
	vmp, err := createVMMProvider(ctx, command)
	if err != nil {
		return fmt.Errorf("create run configure failed: %w", err)
	}

	if err := showVersionAndOSInfo(); err != nil {
		logrus.Warn("cannot get Build version/OS information")
	}

	vmc, err := vmp.GetVMConfigure()
	if err != nil {
		return fmt.Errorf("failed to get vm configure: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	if command.IsSet(define.FlagRestAPIListenAddr) && command.String(define.FlagRestAPIListenAddr) != "" {
		g.Go(func() error {
			return server.NewAPIServer(vmc).Start(ctx)
		})
	}

	g.Go(func() error {
		return vmp.StartNetwork(ctx)
	})

	g.Go(func() error {
		if err = vmp.Create(ctx); err != nil {
			return fmt.Errorf("failed to create vm: %w", err)
		}

		return vmp.Start(ctx)
	})

	g.Go(func() error {
		tcpAddr, err := network.ParseTcpAddr(define.PodmanDefaultListenTcpAddrInGuest)
		if err != nil {
			return fmt.Errorf("failed to parse tcp addr %q: %w", define.PodmanDefaultListenTcpAddrInGuest, err)
		}

		return network.ForwardPodmanAPIOverVSock(ctx, vmc.GVproxyEndpoint, vmc.PodmanInfo.UnixSocksAddr, tcpAddr.Host, uint16(tcpAddr.Port))
	})

	return g.Wait()
}

func setDockerModeParameters(vmc *vmconfig.VMConfig, command *cli.Command) error {
	// Set docker mode
	vmc.Cmdline.Mode = define.RunDockerEngineMode

	if err := useBuiltinRootfs(vmc); err != nil {
		return fmt.Errorf("failed to use builtin podman rootfs: %w", err)
	}

	if err := addContainerStorageDisk(vmc, command); err != nil {
		return fmt.Errorf("failed to add container storage disk: %w", err)
	}

	if err := addPodmanInfo(vmc, command); err != nil {
		return fmt.Errorf("failed to add podman info: %w", err)
	}

	// add user home mount point
	if err := addUserHomeMountPoint(vmc); err != nil {
		return fmt.Errorf("failed to add user home mount point: %w", err)
	}

	return nil
}

func addContainerStorageDisk(vmc *vmconfig.VMConfig, command *cli.Command) error {
	containerStorageDisk, err := filepath.Abs(command.String(define.FlagContainerDataStorage))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	logrus.Infof("in docker mode, container storage disk will be %q", containerStorageDisk)
	vmc.DataDisk = append(vmc.DataDisk, &define.DataDisk{
		IsContainerStorage: true,
		Path:               containerStorageDisk,
	})
	return nil
}

func addPodmanInfo(vmc *vmconfig.VMConfig, command *cli.Command) error {
	path, err := filepath.Abs(command.String(define.FlagListenUnixFile))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	unixAddr := &url.URL{
		Scheme: "unix",
		Host:   "",
		Path:   path,
	}

	vmc.PodmanInfo = define.PodmanInfo{
		UnixSocksAddr: unixAddr.String(),
	}
	return nil
}

func addUserHomeMountPoint(vmc *vmconfig.VMConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("can not get user home directry: %w", err)
	}

	mntStr := fmt.Sprintf("%s:%s", homeDir, homeDir)

	logrus.Infof("in docker-mode, add user home mount point information: %q", mntStr)
	vmc.Mounts = append(vmc.Mounts, filesystem.CmdLineMountToMounts([]string{mntStr})...)

	return nil
}

func useBuiltinRootfs(vmc *vmconfig.VMConfig) error {
	if vmc.RootFS != "" {
		// when vm.RootFS is set, use it directly
		path, err := filepath.Abs(vmc.RootFS)
		if err != nil {
			return err
		}
		vmc.RootFS = path
		logrus.Infof("in docker-mode, use user set rootfs: %q", vmc.RootFS)
		return nil
	}

	dir, err := system.Get3rdDir()
	if err != nil {
		return fmt.Errorf("failed to get 3rd dir: %w", err)
	}
	podmanRootfs := filepath.Join(dir, "linux", "rootfs")
	vmc.RootFS = podmanRootfs
	logrus.Infof("in docker-mode, use builtin podman rootfs: %q", vmc.RootFS)
	return nil
}
