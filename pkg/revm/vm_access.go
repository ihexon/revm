//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
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

func ListPorts(ctx context.Context, session string) ([]define.PortMapping, error) {
	view, err := InspectSession(session)
	if err != nil {
		return nil, err
	}
	if view.Config.Network == define.NetworkTSI {
		return nil, fmt.Errorf("TSI has no port forwarding table: guest listeners bind host ports directly")
	}
	if !view.Active || view.LastRun == nil {
		return nil, fmt.Errorf("session %q is not running", session)
	}
	return gvproxy.ListPorts(ctx, newMachinePathManager(view.LastRun.RuntimeDir).GetGVPCtlSocketFile())
}
