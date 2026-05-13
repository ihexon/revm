//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package machine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"
)

type fakeBackend struct{}

func (fakeBackend) Start(context.Context) error { return nil }
func (fakeBackend) Stop() error                 { return nil }

func TestMachineViewsAreSanitizedAndVersioned(t *testing.T) {
	m := newTestMachine(t)

	guest := m.GuestSpec()
	attach := m.AttachSpec()
	management := m.ManagementView()
	ssh := m.SSHTarget()
	gvproxy := m.GVProxySpec()

	if guest.SchemaVersion != protocol.GuestSpecVersion {
		t.Fatalf("guest spec version = %d, want %d", guest.SchemaVersion, protocol.GuestSpecVersion)
	}
	if attach.SchemaVersion != protocol.AttachSpecVersion {
		t.Fatalf("attach spec version = %d, want %d", attach.SchemaVersion, protocol.AttachSpecVersion)
	}
	if guest.SSH.HostSSHPublicKey != "public-key" {
		t.Fatalf("guest public key = %q", guest.SSH.HostSSHPublicKey)
	}
	if management.Endpoints.SSH != "127.0.0.1:6123" {
		t.Fatalf("management ssh endpoint = %q", management.Endpoints.SSH)
	}
	if !ssh.UseGVProxyTunnel {
		t.Fatalf("ssh target should use gvproxy tunnel")
	}
	if gvproxy.HostSSHForwardAddr != "127.0.0.1:6123" {
		t.Fatalf("gvproxy ssh forward = %q", gvproxy.HostSSHForwardAddr)
	}
	if attach.PrivateKeyFile != "/tmp/host-key" {
		t.Fatalf("attach private key file = %q", attach.PrivateKeyFile)
	}
	if attach.GVPCtlAddr != "unix:///tmp/revm/gvproxy.sock" {
		t.Fatalf("attach gvproxy control = %q", attach.GVPCtlAddr)
	}

	managementJSON, err := json.Marshal(management)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(managementJSON), "PRIVATE-KEY-BODY") || strings.Contains(string(managementJSON), "/tmp/host-key") {
		t.Fatalf("management view leaks private ssh material: %s", managementJSON)
	}

	guestJSON, err := json.Marshal(guest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(guestJSON), "PRIVATE-KEY-BODY") || strings.Contains(string(guestJSON), "/tmp/host-key") {
		t.Fatalf("guest spec leaks host private ssh material: %s", guestJSON)
	}

	attachJSON, err := json.Marshal(attach)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(attachJSON), "PRIVATE-KEY-BODY") {
		t.Fatalf("attach spec leaks private ssh material: %s", attachJSON)
	}
}

func TestMachineViewsCopySlices(t *testing.T) {
	spec := &define.MachineSpec{
		Cmdline: define.Cmdline{
			Args: []string{"before"},
			Envs: []string{"A=B"},
		},
		PodmanInfo: define.PodmanInfo{
			GuestPodmanRunWithEnvs: []string{"P=1"},
		},
	}
	m, err := New(spec, fakeBackend{})
	if err != nil {
		t.Fatal(err)
	}

	guest := m.GuestSpec()
	spec.Cmdline.Args[0] = "after"
	spec.Cmdline.Envs[0] = "A=C"
	spec.PodmanInfo.GuestPodmanRunWithEnvs[0] = "P=2"

	if guest.Cmdline.Args[0] != "before" {
		t.Fatalf("guest args alias machine slice")
	}
	if guest.Cmdline.Envs[0] != "A=B" {
		t.Fatalf("guest envs alias machine slice")
	}
	if guest.Podman.GuestPodmanRunWithEnvs[0] != "P=1" {
		t.Fatalf("guest podman envs alias machine slice")
	}
}

func newTestMachine(t *testing.T) *Machine {
	t.Helper()

	machine, err := New(&define.MachineSpec{
		RunMode:            define.RootFsMode.String(),
		VirtualNetworkMode: define.GVISOR,
		MemoryInMB:         2048,
		Cpus:               4,
		VMCtlAddr:          "unix:///tmp/revm/vmctl.sock",
		GVPCtlAddr:         "unix:///tmp/revm/gvproxy.sock",
		GVPVNetAddr:        "unixgram:///tmp/revm/vnet.sock",
		GVPNotifyAddr:      "unix:///tmp/revm/notify.sock",
		TTY:                true,
		Cmdline: define.Cmdline{
			Bin:     "/bin/echo",
			Args:    []string{"hello"},
			Envs:    []string{"A=B"},
			WorkDir: "/work",
		},
		Mounts: []define.Mount{{
			ReadOnly: true,
			Source:   "/host",
			Target:   "/guest",
			Type:     "virtiofs",
			Tag:      "tag0",
			Opts:     "ro",
			UUID:     "mount-uuid",
		}},
		BlkDevs: []define.BlkDev{{
			UUID:    "disk-uuid",
			Path:    "/host/disk.raw",
			MountTo: "/data",
			FsType:  "ext4",
		}},
		SSHInfo: define.SSHInfo{
			HostSSHPublicKey:         "public-key",
			HostSSHPrivateKey:        "PRIVATE-KEY-BODY",
			HostSSHPrivateKeyFile:    "/tmp/host-key",
			HostSSHProxyListenAddr:   "127.0.0.1:6123",
			GuestSSHServerListenAddr: "0.0.0.0:2222",
			GuestSSHPrivateKeyFile:   "/run/dropbear/private.key",
			GuestSSHAuthorizedKeys:   "/run/dropbear/authorized_keys",
			GuestSSHPidFile:          "/run/dropbear/dropbear.pid",
		},
		PodmanInfo: define.PodmanInfo{
			HostPodmanProxyAddr:      "unix:///tmp/revm/podman.sock",
			GuestPodmanAPIListenAddr: "0.0.0.0:12345",
			GuestPodmanRunWithEnvs:   []string{"HTTP_PROXY=http://proxy"},
		},
	}, fakeBackend{})
	if err != nil {
		t.Fatal(err)
	}
	return machine
}
