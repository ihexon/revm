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
	mc, err := loadMachineConfig(machineConfigFD)
	if err != nil {
		exit(err)
	}

	if _, err = commonpkg.SetupLogger(
		currentLogLevelFromEnv(),
		"krun-runner",
		mc.LogFile,
	); err != nil {
		exit(fmt.Errorf("krun-runner: setup logger: %w", err))
	}

	go monitorOrphan(200 * time.Millisecond)
	// go monitorSignal()

	// run() in a dedicated goroutine with LockOSThread,
	// ensuring all libkrun CGo calls stay on the same OS thread.
	runCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		runCh <- run(context.Background(), mc)
	}()

	if err := <-runCh; err != nil {
		exit(err)
	}
}

func exit(err error) {
	logrus.Error(err)
	os.Exit(1)
}

func monitorOrphan(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if os.Getppid() == 1 {
			exit(fmt.Errorf("parent process exited"))
		}
	}
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

	return vm.Start(ctx)
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

	mc.EnsureRuntime()

	return &mc, nil
}
