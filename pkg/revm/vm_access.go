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
	"linuxvm/pkg/service/management"
	"net/http"
)

func ExecSession(ctx context.Context, session string, opts ExecOptions) error {
	if err := normalizeExecOptions(&opts); err != nil {
		return commanderr.Usage(err)
	}
	if err := validateSessionID(session); err != nil {
		return commanderr.Usage(err)
	}
	workspace, err := sessionWorkspace(session)
	if err != nil {
		return err
	}
	if len(opts.Exec.Argv) == 0 {
		opts.Exec.Argv = []string{"/bin/sh"}
	}
	_, err = executeGuest(ctx, newMachinePathManager(workspace).GetGuestControlSocketFile(), opts)
	return err
}

func portEndpoint(ctx context.Context, session string) (string, error) {
	if err := validateSessionID(session); err != nil {
		return "", commanderr.Usage(err)
	}
	workspace, err := sessionWorkspace(session)
	if err != nil {
		return "", err
	}
	view, err := fetchManagementView(ctx, workspace)
	if err != nil {
		return "", err
	}
	if view.NetworkMode != string(define.GVISOR) {
		return "", fmt.Errorf("port operations require gvisor network")
	}
	return view.Endpoints.GVProxyAPI, nil
}

func ListPorts(ctx context.Context, session string) ([]define.PortMapping, error) {
	endpoint, err := portEndpoint(ctx, session)
	if err != nil {
		return nil, err
	}
	return gvproxy.ListPorts(ctx, endpoint)
}

func UpdatePorts(ctx context.Context, session string, exports, unexports []define.PortForward) error {
	if err := validatePortUpdateSet(unexports, "unexport"); err != nil {
		return commanderr.Usage(err)
	}
	if err := validatePortUpdateSet(exports, "export"); err != nil {
		return commanderr.Usage(err)
	}
	endpoint, err := portEndpoint(ctx, session)
	if err != nil {
		return err
	}
	for _, port := range unexports {
		if err := gvproxy.UnexposePort(ctx, endpoint, port); err != nil {
			return err
		}
	}
	for _, port := range exports {
		if err := gvproxy.ExposePort(ctx, endpoint, port); err != nil {
			return err
		}
	}
	return nil
}

func fetchManagementView(ctx context.Context, workspaceDirPath string) (management.VMConfigView, error) {
	return fetchManagementJSON[management.VMConfigView](ctx, workspaceDirPath, "/v2/vmconfig", "vm config")
}

func fetchManagementJSON[T any](ctx context.Context, workspaceDirPath, path, label string) (T, error) {
	var value T
	vmctlAddr := newMachinePathManager(workspaceDirPath).GetVMCtlSocketFile()
	body, status, err := network.UnixRequest(ctx, vmctlAddr, http.MethodGet, path, nil)
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
