//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
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
	workspace, err := sessionWorkspace(session)
	if err != nil {
		return nil, err
	}
	return gvproxy.ListPorts(ctx, newMachinePathManager(workspace).GetGVPCtlSocketFile())
}
