//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	commonpkg "linuxvm/pkg/log"
	"os"
	"runtime"
	"strings"
	"time"

	"linuxvm/cmd/krun-runner/pkg/libkrun"

	"github.com/sirupsen/logrus"
)

const machineConfigFD = 3

func main() {
	runtime.LockOSThread()

	if err := runMain(); err != nil {
		logrus.Fatal(err)
	}
}

func runMain() error {
	mc, err := loadMachineConfig(machineConfigFD)
	if err != nil {
		return err
	}

	if err := commonpkg.SetupBasicLoggerWithStageAndFile(
		currentLogLevelFromEnv(),
		"krun-runner",
		mc.LogFilePath,
	); err != nil {
		return fmt.Errorf("krun-runner: setup logger: %w", err)
	}

	orphanCh, stopMonitor := startOrphanMonitor(200 * time.Millisecond)
	defer stopMonitor()

	runCh := make(chan error, 1)
	go func() {
		runCh <- run(context.Background(), mc)
	}()

	select {
	case err := <-runCh:
		return err
	case err := <-orphanCh:
		return err
	}
}

func startOrphanMonitor(interval time.Duration) (<-chan error, func()) {
	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if os.Getppid() == 1 {
					select {
					case errCh <- define.ErrParentProcessExit:
					default:
					}
					return
				}
			}
		}
	}()

	return errCh, func() { close(done) }
}

func currentLogLevelFromEnv() string {
	level := strings.ToLower(os.Getenv(define.EnvLogLevel))
	if level == "" {
		return "info"
	}
	return level
}

func run(ctx context.Context, mc *define.Machine) error {
	vm := libkrun.NewLibkrunVM(mc)
	if err := vm.Create(ctx); err != nil {
		return fmt.Errorf("krun-runner: create: %w", err)
	}

	// Start() 调用 krun_start_enter()，成功时 libkrun 调用 exit()
	// 此进程会被直接终止，主进程通过 waitpid 感知退出
	if err := vm.Start(ctx); err != nil {
		return fmt.Errorf("krun-runner: start: %w", err)
	}
	return nil
}

func loadMachineConfig(fd uintptr) (*define.Machine, error) {
	configFile := os.NewFile(fd, "machine-config")
	if configFile == nil {
		return nil, fmt.Errorf("krun-runner: fd %d not available", fd)
	}
	defer configFile.Close()

	var mc define.Machine
	if err := json.NewDecoder(configFile).Decode(&mc); err != nil {
		return nil, fmt.Errorf("krun-runner: decode config: %w", err)
	}

	// 重建不可序列化字段
	mc.EnsureRuntime()

	return &mc, nil
}
