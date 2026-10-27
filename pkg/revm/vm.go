//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/network"
	"linuxvm/pkg/podmanproxy"
	"linuxvm/pkg/service/management"

	"github.com/sirupsen/logrus"
)

// The CLI process is the VM. Returning ends that process, including every
// native thread and host service. The cleanup helper owns post-exit files.
// Run is therefore a one-shot CLI operation, not an embeddable VM lifetime.
type VM struct {
	cfg  *VMOptions
	spec *define.MachineSpec
	stop context.CancelCauseFunc
}

func buildTimeInfo() string {
	version, commit, date := define.Version, define.CommitID, define.BuildDate
	if version == "" {
		version = "unknown"
	}
	if commit == "" {
		commit = "unknown"
	}
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("%s-%s-%s", version, commit, date)
}

func RunVM(ctx context.Context, cfg VMOptions) (retErr error) {
	cfg, err := normalizeVMOptions(cfg)
	if err != nil {
		return commanderr.Usage(err)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)
	if err := libkrun.CheckHostSupport(); err != nil {
		return err
	}
	lease, err := beginRun(ctx, &cfg)
	if err != nil {
		return err
	}
	if err = startCleanup(ctx, lease, cancel); err != nil {
		return lease.diagnose(errors.Join(err, lease.close(lease.finish(nil, err))))
	}
	var result *execution.Result
	defer func() {
		retErr = errors.Join(retErr, lease.finish(result, retErr))
		retErr = lease.diagnose(retErr)
	}()
	logFile, err := setupRunLogging(cfg.LogLevel, lease.logDir)
	if err != nil {
		return err
	}
	defer releaseRunLog(logFile)
	logrus.Infof("revm build info: %s", buildTimeInfo())
	logrus.Infof("start virtualMachine, full cmdline: %q", os.Args)
	spec, err := buildMachine(ctx, cfg, lease)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}
	paths := newMachinePathManager(lease.record.RuntimeDir)
	for link, target := range map[string]string{cfg.ManageAPIFile: paths.GetVMCtlSocketFile(), cfg.PodmanProxyAPIFile: paths.GetPodmanSocketFile()} {
		if link != "" {
			if err = createSymlink(target, link); err != nil {
				return err
			}
		}
	}
	lease.record.Status = "running"
	if err = writeJSONAtomic(lease.resultPath, lease.record); err != nil {
		return err
	}
	// Native libraries create sockets in TMPDIR. Keep them inside the instance
	// directory so process exit never leaves those paths outside cleanup ownership.
	if err = os.Setenv("TMPDIR", lease.record.RuntimeDir); err != nil {
		return err
	}
	vm := &VM{cfg: &cfg, spec: spec}
	result, retErr = vm.Run(ctx)
	return retErr
}

func (vm *VM) Run(ctx context.Context) (*execution.Result, error) {
	ctx, vm.stop = context.WithCancelCause(ctx)
	defer vm.stop(context.Canceled)
	errorsCh := make(chan error, 4)
	start := func(service func(context.Context) error) {
		go func() {
			if err := service(ctx); err != nil && ctx.Err() == nil {
				errorsCh <- err
			}
		}()
	}
	start(func(ctx context.Context) error {
		server, err := management.NewServer(vm)
		if err != nil {
			return err
		}
		return server.Start(ctx)
	})
	networkReady := make(chan struct{})
	ready := sync.OnceFunc(func() { close(networkReady) })
	if vm.spec.VirtualNetworkMode == define.TSI {
		ready()
	} else {
		start(func(ctx context.Context) error { return gvproxy.Run(ctx, vm.GVProxySpec(), ready) })
	}
	if vm.cfg.RunMode == define.ContainerMode {
		start(func(ctx context.Context) error {
			socket, err := network.ParseUnixAddr(vm.spec.PodmanInfo.HostPodmanProxyAddr)
			if err != nil {
				return err
			}
			transport, err := network.ParseUnixAddr(vm.spec.PodmanInfo.HostPodmanTransportAddr)
			if err != nil {
				return err
			}
			return podmanproxy.ServeHost(ctx, socket.Path, transport.Path)
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-networkReady:
		}
		logrus.Info("boot virtual machine")
		err := libkrun.Run(vm.spec)
		if err == nil {
			err = errors.New("VM stopped without a command result; exit status unknown")
		}
		errorsCh <- err
	}()
	type completed struct {
		result execution.Result
		err    error
	}
	commandDone := make(chan completed, 1)
	if vm.cfg.RunMode == define.RootFsMode {
		go func() {
			result, err := executeGuest(ctx, newMachinePathManager(vm.spec.WorkspaceDir).GetGuestControlSocketFile(), vm.cfg.ExecOptions)
			commandDone <- completed{result, err}
		}()
	}
	select {
	case result := <-commandDone:
		if result.result.Code == nil && result.result.Signal == "" {
			return nil, result.err
		}
		return &result.result, result.err
	case err := <-errorsCh:
		return nil, err
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (vm *VM) RequestStop(context.Context) error {
	vm.stop(&execution.Canceled{Signal: "SIGTERM"})
	return nil
}
