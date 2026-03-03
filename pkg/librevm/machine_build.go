//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"

	"github.com/sirupsen/logrus"
)

// buildMachine converts Config directly into define.Machine.
// The returned cleanup func releases all resources (file lock, log file,
// workspace directory). Caller must always invoke it.
func buildMachine(ctx context.Context, cfg Config, workspacePath string) (mc *define.Machine, cleanup func(), retErr error) {
	runMode := define.ContainerMode
	if cfg.Mode == ModeRootfs {
		runMode = define.RootFsMode
	}

	vmc := newMachineBuilder(runMode)

	// cleanups 按 LIFO 顺序执行
	var cleanups []func()
	cleanup = func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	defer func() {
		if retErr != nil {
			cleanup()
			cleanup = nil
		}
	}()

	if err := vmc.setupWorkspace(workspacePath); err != nil {
		return nil, nil, fmt.Errorf("setup workspace: %w", err)
	}
	cleanups = append(cleanups,
		func() { _ = vmc.fileLock.Unlock(); _ = os.Remove(workspacePath + ".lock") },
		func() { _ = os.RemoveAll(workspacePath) },
	)
	pathMgr := newMachinePathManager(vmc.WorkspacePath)

	if err := vmc.configureSSH(pathMgr); err != nil {
		return nil, nil, fmt.Errorf("generate ssh config: %w", err)
	}
	logFile, err := vmc.setupLogLevel(cfg.LogLevel)
	if err != nil {
		return nil, nil, fmt.Errorf("setup log level: %w", err)
	}
	cleanups = append(cleanups, func() { logrus.SetOutput(os.Stderr); _ = logFile.Close() })
	if err := vmc.withResources(cfg.MemoryMB, uint8(cfg.CPUs)); err != nil {
		return nil, nil, fmt.Errorf("set resources: %w", err)
	}
	if err := vmc.configureNetwork(ctx, define.VNetMode(cfg.Network), pathMgr); err != nil {
		return nil, nil, fmt.Errorf("configure network: %w", err)
	}
	if cfg.Proxy {
		if err := vmc.applySystemProxy(); err != nil {
			return nil, nil, fmt.Errorf("apply system proxy: %w", err)
		}
	}

	if cfg.Rootfs != "" {
		if err := vmc.withUserProvidedRootfs(ctx, cfg.Rootfs); err != nil {
			return nil, nil, err
		}
	} else {
		if err := vmc.withBuiltInAlpineRootfs(ctx, pathMgr); err != nil {
			return nil, nil, err
		}
	}

	switch runMode {
	case define.RootFsMode:
		workDir := cfg.WorkDir
		if workDir == "" {
			workDir = "/"
		}
		bin := cfg.Command[0]
		var args []string
		if len(cfg.Command) > 1 {
			args = cfg.Command[1:]
		}
		if err := vmc.setupCmdLine(workDir, bin, args, cfg.Env); err != nil {
			return nil, nil, fmt.Errorf("setup cmdline: %w", err)
		}
	case define.ContainerMode:
		if err := vmc.withMountUserHome(ctx); err != nil {
			return nil, nil, fmt.Errorf("mount user home: %w", err)
		}
		if err := vmc.configurePodman(ctx, pathMgr); err != nil {
			return nil, nil, fmt.Errorf("configure podman: %w", err)
		}

		diskPath := pathMgr.GetBuiltInContainerStorageDiskPathInWorkspace()
		if cfg.ContainerDisk != "" {
			diskPath = cfg.ContainerDisk
		}
		if err := vmc.configureContainerRAWDisk(ctx, diskPath); err != nil {
			return nil, nil, fmt.Errorf("setup container disk: %w", err)
		}
	}

	if len(cfg.Disks) > 0 {
		if err := vmc.withUserProvidedStorageRAWDisk(ctx, cfg.Disks); err != nil {
			return nil, nil, fmt.Errorf("attach raw disks: %w", err)
		}
	}
	if len(cfg.Mounts) > 0 {
		if err := vmc.withUserProvidedMounts(cfg.Mounts); err != nil {
			return nil, nil, fmt.Errorf("setup mounts: %w", err)
		}
	}
	if err := vmc.configureGuestAgent(ctx, pathMgr); err != nil {
		return nil, nil, fmt.Errorf("configure guest agent: %w", err)
	}
	if err := vmc.configureVMCtlAPI(pathMgr); err != nil {
		return nil, nil, fmt.Errorf("configure vmctl API: %w", err)
	}

	return &vmc.Machine, cleanup, nil
}
