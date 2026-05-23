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
	libarchive_go "linuxvm/pkg/libarchive"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"linuxvm/pkg/service/management"
	sshsvc "linuxvm/pkg/service/ssh"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
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

	if !normalizedCfg.PortList && len(normalizedCfg.PortForwards) == 0 && len(normalizedCfg.PortUnforwards) == 0 && normalizedCfg.RootfsExport == "" {
		return fmt.Errorf("no control operation requested")
	}
	if len(normalizedCfg.Command) > 0 {
		return fmt.Errorf("control operations cannot be combined with an attach command")
	}
	if normalizedCfg.PortList {
		_, err := ListPorts(ctx, cfg)
		return err
	}
	if normalizedCfg.RootfsExport != "" {
		return ExportRootfs(ctx, normalizedCfg)
	}

	return updatePortForwards(ctx, normalizedCfg)
}

func ExportRootfs(ctx context.Context, normalizedCfg Config) error {
	if normalizedCfg.RootfsExport == "" {
		return fmt.Errorf("rootfs export path must not be empty")
	}
	rootfsDir := newMachinePathManager(getSessionDir(normalizedCfg.SessionID)).GetRootfsDir()
	if info, err := os.Stat(rootfsDir); err != nil {
		return fmt.Errorf("stat session rootfs %q: %w", rootfsDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("session rootfs %q is not a directory", rootfsDir)
	}

	outputPath, err := filepath.Abs(filepath.Clean(normalizedCfg.RootfsExport))
	if err != nil {
		return fmt.Errorf("resolve rootfs export path: %w", err)
	}
	rootfsAbs, err := filepath.Abs(filepath.Clean(rootfsDir))
	if err != nil {
		return fmt.Errorf("resolve session rootfs path: %w", err)
	}
	if pathWithin(outputPath, rootfsAbs) {
		return fmt.Errorf("rootfs export path %q must not be inside session rootfs %q", outputPath, rootfsAbs)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create rootfs export directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary rootfs export: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close rootfs export: %w", err)
	}
	if err := libarchive_go.NewArchiver().
		WithArchiveFilePath(tmpPath).
		SetChdir(rootfsAbs).
		ModeC(ctx); err != nil {
		return fmt.Errorf("archive rootfs: %w", err)
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		return fmt.Errorf("chmod rootfs export: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("move rootfs export into place: %w", err)
	}
	cleanup = false
	logrus.Infof("exported rootfs %q to %q", rootfsAbs, outputPath)
	return nil
}

func pathWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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
	sshTarget := sshTargetFromAttachSpec(attachSpec)

	if cfg.PTY {
		return attachShell(ctx, sshTarget)
	}
	return attachRun(ctx, sshTarget, cfg.Command...)
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

// attachRun executes a command in the attached VM session over SSH.
// If cmdline is empty, it runs /bin/sh.
func attachRun(ctx context.Context, sshTarget sshsvc.Target, cmdline ...string) error {
	if len(cmdline) == 0 {
		cmdline = []string{filepath.Join("/", "bin", "sh")}
	}

	client, err := sshsvc.MakeSSHClient(ctx, sshTarget)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	return client.Run(ctx, shellescape.QuoteCommand(cmdline))
}

// attachShell starts an interactive shell in the attached VM session over SSH.
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
	client, err := sshsvc.MakeSSHClient(ctx, vm.runtime.view.SSHTarget())
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
	client, err := sshsvc.MakeSSHClient(ctx, vm.runtime.view.SSHTarget())
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
