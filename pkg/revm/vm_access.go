//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	guestcontrol "linuxvm/pkg/service/guestcontrol"
	"linuxvm/pkg/service/management"
	sshsvc "linuxvm/pkg/service/ssh"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// Attach resolves the attach configuration and connects to an existing VM
// session without building or starting a virtual machine.
func Attach(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}
	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return fmt.Errorf("resolve defaults: %w", err)
	}
	if normalizedCfg.RunMode != ModeAttach {
		return fmt.Errorf("attach requires run mode %q, got %q", ModeAttach, normalizedCfg.RunMode)
	}
	logFile, err := setupRunLogging(normalizedCfg)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	defer releaseRunLog(logFile)

	return attach(ctx, normalizedCfg)
}

func Control(ctx context.Context, cfg *Config) (retErr error) {
	if cfg == nil {
		return fmt.Errorf("config must not be nil")
	}

	normalizedCfg, err := NormalizeConfig(*cfg)
	if err != nil {
		return fmt.Errorf("resolve defaults: %w", err)
	}
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
	if len(normalizedCfg.Command) > 0 {
		return fmt.Errorf("control operations cannot be combined with an attach command")
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
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}
	if normalizedCfg.RunMode != ModeControl {
		return nil, fmt.Errorf("port list requires run mode %q, got %q", ModeControl, normalizedCfg.RunMode)
	}
	view, err := fetchManagementView(ctx, getSessionDir(normalizedCfg.SessionID))
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
	view, err := fetchManagementView(ctx, getSessionDir(normalizedCfg.SessionID))
	if err != nil {
		return err
	}
	if view.NetworkMode != string(define.GVISOR) {
		return fmt.Errorf("port updates require network %q, got %q", define.GVISOR, view.NetworkMode)
	}
	if view.Endpoints.GVProxyAPI == "" {
		return fmt.Errorf("gvproxy API endpoint is empty")
	}
	if err := validatePortUpdateSet(normalizedCfg.PortUnforwards, view.Endpoints.SSH, "unexport"); err != nil {
		return err
	}
	if err := validatePortUpdateSet(normalizedCfg.PortForwards, view.Endpoints.SSH, "export"); err != nil {
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

func attach(ctx context.Context, cfg Config) error {
	attachSpec, err := fetchAttachSpec(ctx, getSessionDir(cfg.SessionID))
	if err != nil {
		return err
	}
	controlTarget := guestcontrol.Target{CID: attachSpec.GuestControlCID, Port: attachSpec.GuestControlPort}
	if attachSpec.GuestControlSocket != "" {
		if addr, parseErr := network.ParseUnixAddr(attachSpec.GuestControlSocket); parseErr == nil {
			controlTarget.UnixSocket = addr.Path
		}
	}
	defaults := guestcontrol.DefaultTarget()
	if controlTarget.CID == 0 {
		controlTarget.CID = defaults.CID
	}
	if controlTarget.Port == 0 {
		controlTarget.Port = defaults.Port
	}

	if cfg.PTY {
		return attachShell(ctx, sshTargetFromAttachSpec(attachSpec))
	}
	return attachRun(ctx, controlTarget, cfg.Command...)
}

func fetchAttachSpec(ctx context.Context, workspaceDirPath string) (protocol.AttachSpec, error) {
	spec, err := fetchManagementJSON[protocol.AttachSpec](ctx, workspaceDirPath, "/v2/attach", "attach spec")
	if err != nil {
		return protocol.AttachSpec{}, err
	}
	if spec.SchemaVersion != protocol.AttachSpecVersion {
		return protocol.AttachSpec{}, fmt.Errorf("unsupported attach spec version: %d", spec.SchemaVersion)
	}
	return spec, nil
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

// attachRun executes a command in the attached VM session over GuestControl.
// If cmdline is empty, it runs /bin/sh.
func attachRun(ctx context.Context, controlTarget guestcontrol.Target, cmdline ...string) error {
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}

	proc, err := guestcontrol.GuestExec(ctx, controlTarget, cmdline[0], cmdline[1:]...)
	if err != nil {
		return err
	}
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stderr, proc.StderrPipeReader)
		close(stderrDone)
	}()
	_, copyErr := io.Copy(os.Stdout, proc.StdoutPipeReader)
	if copyErr != nil {
		return copyErr
	}
	<-stderrDone
	if err := <-proc.ErrChan; err != nil {
		return err
	}
	return nil
}

// attachShell starts an interactive shell in the attached VM session over SSH
// as the compatibility path because GuestControl PTY support is separate.
func attachShell(ctx context.Context, sshTarget sshsvc.Target) error {
	client, err := sshsvc.MakeSSHClient(ctx, sshTarget)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Shell(ctx)
}

// Exec runs a command inside the guest VM and returns its combined stdout
// output. It blocks until the command completes.
func (vm *VM) Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	proc, err := guestcontrol.GuestExec(ctx, vm.runtime.view.GuestControlTarget(), name, args...)
	if err != nil {
		return nil, err
	}
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, proc.StderrPipeReader)
		close(stderrDone)
	}()
	out, err := io.ReadAll(proc.StdoutPipeReader)
	if err != nil {
		return nil, err
	}
	<-stderrDone
	if err := <-proc.ErrChan; err != nil {
		return nil, err
	}
	return out, nil
}

// ExecWith runs a command inside the guest VM with custom I/O streams.
// It blocks until the command completes.
func (vm *VM) ExecWith(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer,
	name string, args ...string) error {
	proc, err := guestcontrol.GuestExecWith(ctx, vm.runtime.view.GuestControlTarget(), stdin, name, args...)
	if err != nil {
		return err
	}
	copyErrCh := make(chan error, 2)
	go func() { _, err := io.Copy(stdout, proc.StdoutPipeReader); copyErrCh <- err }()
	go func() { _, err := io.Copy(stderr, proc.StderrPipeReader); copyErrCh <- err }()
	var copyErr error
	for i := 0; i < 2; i++ {
		if err := <-copyErrCh; err != nil && copyErr == nil {
			copyErr = err
		}
	}
	if err := <-proc.ErrChan; err != nil && copyErr == nil {
		copyErr = err
	}
	return copyErr
}

// Shell opens an interactive shell session to the guest VM.
// It requires a TTY on the host side.
func (vm *VM) Shell(ctx context.Context) error {
	client, err := sshsvc.MakeSSHClient(ctx, vm.runtime.view.SSHTarget())
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Shell(ctx)
}

// SSHEndpoint returns the configured guest SSH address (host:port).
// It does not wait for SSH readiness; callers should retry the connection.
func (vm *VM) SSHEndpoint(ctx context.Context) (string, error) {
	return vm.runtime.view.SSHTarget().GuestSSHServerListenAddr, nil
}

// PodmanEndpoint returns the configured host-side Podman unix socket address.
// It does not wait for Podman readiness; callers should retry the connection.
func (vm *VM) PodmanEndpoint(ctx context.Context) (string, error) {
	return vm.runtime.view.PodmanHostProxyAddr(), nil
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
