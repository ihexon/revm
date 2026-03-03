//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	sshsvc "linuxvm/pkg/service/ssh"
	"net/http"
	"path/filepath"

	"al.essio.dev/pkg/shellescape"
)

// AttachedVM represents a running VM session that can be attached over SSH.
type AttachedVM struct {
	machine *define.Machine
}

// Attach resolves a running VM session by name and returns an attach handle.
func Attach(ctx context.Context, sessionName string) (*AttachedVM, error) {
	if sessionName == "" {
		return nil, fmt.Errorf("session name must not be empty")
	}

	workspace := workspacePathForSession(sessionName)
	ignAddr := ignitionSockPath(workspace)

	client := network.NewUnixClient(ignAddr)
	defer client.Close()

	resp, err := client.Get(define.RestAPIVMConfigURL).Do(ctx) //nolint:bodyclose
	if err != nil {
		return nil, fmt.Errorf("fetch vm config: %w", err)
	}
	defer network.CloseResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ignition server returned status %d", resp.StatusCode)
	}

	var vmc define.Machine
	if err := json.NewDecoder(resp.Body).Decode(&vmc); err != nil {
		return nil, fmt.Errorf("decode vm config: %w", err)
	}

	return &AttachedVM{machine: &vmc}, nil
}

// Run executes a command in the attached VM session over SSH.
// If cmdline is empty, it runs /bin/sh.
func (a *AttachedVM) Run(ctx context.Context, cmdline ...string) error {
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}

	sshClient, err := sshsvc.MakeSSHClient(ctx, a.machine)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	return sshClient.Run(ctx, shellescape.QuoteCommand(cmdline))
}

// Shell starts an interactive shell in the attached VM session over SSH.
func (a *AttachedVM) Shell(ctx context.Context) error {
	sshClient, err := sshsvc.MakeSSHClient(ctx, a.machine)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	return sshClient.Shell(ctx)
}
