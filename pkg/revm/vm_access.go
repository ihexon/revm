//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	sshsvc "linuxvm/pkg/service/ssh"
	"net/http"
	"path/filepath"

	"al.essio.dev/pkg/shellescape"
)

type attachedVM struct {
	sshTarget sshsvc.Target
}

func attachWorkspaceDir(ctx context.Context, workspaceDirPath string) (*attachedVM, error) {
	attachSpec, err := fetchAttachSpec(ctx, workspaceDirPath)
	if err != nil {
		return nil, err
	}

	return &attachedVM{sshTarget: sshTargetFromAttachSpec(attachSpec)}, nil
}

// Attach connects to the existing VM session represented by vm.
// It does not build or start a virtual machine.
func (vm *VM) Attach(ctx context.Context) error {
	if vm == nil || vm.cfg == nil {
		return fmt.Errorf("vm must not be nil")
	}
	if vm.sessionDir == "" {
		return fmt.Errorf("session directory must not be empty")
	}

	attached, err := attachWorkspaceDir(ctx, vm.sessionDir)
	if err != nil {
		return err
	}

	if vm.cfg.PTY {
		return attached.shell(ctx)
	}
	return attached.run(ctx, vm.cfg.Command...)
}

func fetchAttachSpec(ctx context.Context, workspaceDirPath string) (protocol.AttachSpec, error) {
	vmctlAddr := newMachinePathManager(workspaceDirPath).GetVMCtlSocketFile()
	client := network.NewUnixClient(vmctlAddr)
	defer client.Close()

	body, status, err := client.Get("/v2/attach").DoAndRead(ctx)
	if err != nil {
		return protocol.AttachSpec{}, fmt.Errorf("fetch attach spec: %w", err)
	}
	if status != http.StatusOK {
		return protocol.AttachSpec{}, fmt.Errorf("management API returned status %d", status)
	}

	var spec protocol.AttachSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return protocol.AttachSpec{}, fmt.Errorf("decode attach spec: %w", err)
	}
	if spec.SchemaVersion != protocol.AttachSpecVersion {
		return protocol.AttachSpec{}, fmt.Errorf("unsupported attach spec version: %d", spec.SchemaVersion)
	}
	return spec, nil
}

func sshTargetFromAttachSpec(spec protocol.AttachSpec) sshsvc.Target {
	return sshsvc.Target{
		User:                     spec.User,
		PrivateKeyFile:           spec.PrivateKeyFile,
		UseGVProxyTunnel:         spec.UseGVProxyTunnel,
		GVPCtlAddr:               spec.GVPCtlAddr,
		GuestSSHServerListenAddr: spec.GuestSSHServerListenAddr,
		GuestTunnelHost:          spec.GuestTunnelHost,
	}
}

// Run executes a command in the attached VM session over SSH.
// If cmdline is empty, it runs /bin/sh.
func (a *attachedVM) run(ctx context.Context, cmdline ...string) error {
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}

	client, err := sshsvc.MakeSSHClient(ctx, a.sshTarget)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Run(ctx, shellescape.QuoteCommand(cmdline))
}

// Shell starts an interactive shell in the attached VM session over SSH.
func (a *attachedVM) shell(ctx context.Context) error {
	client, err := sshsvc.MakeSSHClient(ctx, a.sshTarget)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Shell(ctx)
}

// Exec runs a command inside the guest VM and returns its combined stdout
// output. It blocks until the command completes.
func (vm *VM) Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine.SSHTarget())
	if err != nil {
		return nil, fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Output(ctx, shellescape.QuoteCommand(append([]string{name}, args...)))
}

// ExecWith runs a command inside the guest VM with custom I/O streams.
// It blocks until the command completes.
func (vm *VM) ExecWith(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer,
	name string, args ...string) error {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine.SSHTarget())
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.RunWith(ctx,
		shellescape.QuoteCommand(append([]string{name}, args...)),
		stdin, stdout, stderr)
}

// Shell opens an interactive shell session to the guest VM.
// It requires a TTY on the host side.
func (vm *VM) Shell(ctx context.Context) error {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine.SSHTarget())
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Shell(ctx)
}

// SSHEndpoint returns the configured guest SSH address (host:port).
// It does not wait for SSH readiness; callers should retry the connection.
func (vm *VM) SSHEndpoint(ctx context.Context) (string, error) {
	return vm.machine.SSHTarget().GuestSSHServerListenAddr, nil
}

// PodmanEndpoint returns the configured host-side Podman unix socket address.
// It does not wait for Podman readiness; callers should retry the connection.
func (vm *VM) PodmanEndpoint(ctx context.Context) (string, error) {
	return vm.machine.PodmanHostProxyAddr(), nil
}

// ExecOutput is a convenience that runs Exec and returns stdout as a string,
// trimming trailing whitespace.
func (vm *VM) ExecOutput(ctx context.Context, name string, args ...string) (string, error) {
	out, err := vm.Exec(ctx, name, args...)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimRight(out, " \t\r\n")), nil
}
