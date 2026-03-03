//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	sshsvc "linuxvm/pkg/service/ssh"

	"al.essio.dev/pkg/shellescape"
)

// Exec runs a command inside the guest VM and returns its combined stdout
// output. It blocks until the command completes.
func (vm *VM) Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine)
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
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine)
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
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Shell(ctx)
}

// SSHEndpoint blocks until the guest SSH server is ready and returns the
// SSH address (host:port) suitable for direct connections.
func (vm *VM) SSHEndpoint(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-vm.machine.Readiness.SSHReady:
		return vm.machine.SSHInfo.GuestSSHServerListenAddr, nil
	}
}

// PodmanEndpoint blocks until the guest Podman API proxy is ready and
// returns the host-side unix socket address.
func (vm *VM) PodmanEndpoint(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-vm.machine.Readiness.PodmanReady:
		return vm.machine.PodmanInfo.HostPodmanProxyAddr, nil
	}
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
