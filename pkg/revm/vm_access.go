//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	guestcontrol "linuxvm/pkg/service/guestcontrol"
	"linuxvm/pkg/service/management"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
)

// execSession resolves the execution configuration and connects to an existing VM
// session without building or starting a virtual machine.
func execSession(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}
	normalizedCfg := *cfg
	if normalizedCfg.RunMode != ModeExec {
		return fmt.Errorf("exec requires run mode %q, got %q", ModeExec, normalizedCfg.RunMode)
	}
	logFile, err := setupRunLogging(normalizedCfg)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	defer releaseRunLog(logFile)

	return execInSession(ctx, normalizedCfg)
}

func controlSession(ctx context.Context, cfg *Config) (retErr error) {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}

	normalizedCfg := *cfg
	if normalizedCfg.RunMode != ModeControl {
		return fmt.Errorf("control operations require run mode %q, got %q", ModeControl, normalizedCfg.RunMode)
	}
	logFile, err := setupRunLogging(normalizedCfg)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	defer func() {
		if retErr != nil {
			logrus.Error(retErr)
		}
		releaseRunLog(logFile)
	}()

	logrus.Infof("revm build info: %s", buildTimeInfo())
	logrus.Infof("control command, full cmdline: %q", os.Args)

	if !normalizedCfg.PortList && len(normalizedCfg.PortForwards) == 0 && len(normalizedCfg.PortUnforwards) == 0 {
		return fmt.Errorf("no control operation requested")
	}
	if len(normalizedCfg.Exec.Argv) > 0 {
		return fmt.Errorf("control operations cannot be combined with an exec command")
	}
	if normalizedCfg.PortList {
		_, err := ListPorts(ctx, cfg)
		return err
	}
	return updatePortForwards(ctx, normalizedCfg)
}

func ListPorts(ctx context.Context, cfg *Config) ([]define.PortMapping, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return nil, commanderr.Usage(err)
	}
	if normalizedCfg.RunMode != ModeControl {
		return nil, fmt.Errorf("port list requires run mode %q, got %q", ModeControl, normalizedCfg.RunMode)
	}
	workspace, err := sessionWorkspace(normalizedCfg.SessionID)
	if err != nil {
		return nil, err
	}
	view, err := fetchManagementView(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if view.NetworkMode != string(define.GVISOR) {
		return nil, fmt.Errorf("port list requires network %q, got %q", define.GVISOR, view.NetworkMode)
	}
	if view.Endpoints.GVProxyAPI == "" {
		return nil, fmt.Errorf("gvproxy API endpoint is empty")
	}
	return gvproxy.ListPorts(ctx, view.Endpoints.GVProxyAPI)
}

func updatePortForwards(ctx context.Context, normalizedCfg Config) error {
	workspace, err := sessionWorkspace(normalizedCfg.SessionID)
	if err != nil {
		return err
	}
	view, err := fetchManagementView(ctx, workspace)
	if err != nil {
		return err
	}
	if view.NetworkMode != string(define.GVISOR) {
		return fmt.Errorf("port updates require network %q, got %q", define.GVISOR, view.NetworkMode)
	}
	if view.Endpoints.GVProxyAPI == "" {
		return fmt.Errorf("gvproxy API endpoint is empty")
	}
	if err := validatePortUpdateSet(normalizedCfg.PortUnforwards, "unexport"); err != nil {
		return err
	}
	if err := validatePortUpdateSet(normalizedCfg.PortForwards, "export"); err != nil {
		return err
	}

	for _, forward := range normalizedCfg.PortUnforwards {
		if err := gvproxy.UnexposePort(ctx, view.Endpoints.GVProxyAPI, forward); err != nil {
			return fmt.Errorf("unexport %s: %w", portForwardLocal(forward), err)
		}
	}
	for _, forward := range normalizedCfg.PortForwards {
		if err := gvproxy.ExposePort(ctx, view.Endpoints.GVProxyAPI, forward); err != nil {
			return fmt.Errorf("export %s -> %s: %w", portForwardLocal(forward), portForwardRemote(forward), err)
		}
	}
	return nil
}

func execInSession(ctx context.Context, cfg Config) error {
	workspace, err := sessionWorkspace(cfg.SessionID)
	if err != nil {
		return err
	}
	controlTarget := guestcontrol.Target{UnixSocket: newMachinePathManager(workspace).GetGuestControlSocketFile()}

	if len(cfg.Exec.Argv) == 0 {
		cfg.Exec.Argv = []string{"/bin/sh"}
	}
	done := make(chan error, 1)
	go func() { _, err := executeGuest(ctx, controlTarget, cfg); done <- err }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func fetchManagementView(ctx context.Context, workspaceDirPath string) (management.VMConfigView, error) {
	return fetchManagementJSON[management.VMConfigView](ctx, workspaceDirPath, "/v2/vmconfig", "vm config")
}

func fetchManagementJSON[T any](ctx context.Context, workspaceDirPath, path, label string) (T, error) {
	var value T
	vmctlAddr := newMachinePathManager(workspaceDirPath).GetVMCtlSocketFile()
	client := network.NewUnixClient(vmctlAddr)
	defer client.Close()

	body, status, err := client.Get(path).DoAndRead(ctx)
	if err != nil {
		return value, fmt.Errorf("fetch %s: %w", label, err)
	}
	if status != http.StatusOK {
		return value, fmt.Errorf("management API returned status %d", status)
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return value, fmt.Errorf("decode %s: %w", label, err)
	}
	return value, nil
}
