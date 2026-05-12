//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package runtimecfg

import (
	"encoding/json"
	"strings"
	"testing"

	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"
)

func TestServiceSpecsAreSanitizedAndVersioned(t *testing.T) {
	machine := &define.Machine{
		MachineSpec: define.MachineSpec{
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
		},
	}

	specs := NewServiceSpecs(machine)

	if specs.Guest.SchemaVersion != protocol.GuestSpecVersion {
		t.Fatalf("guest spec version = %d, want %d", specs.Guest.SchemaVersion, protocol.GuestSpecVersion)
	}
	if specs.Guest.SSH.HostSSHPublicKey != "public-key" {
		t.Fatalf("guest public key = %q", specs.Guest.SSH.HostSSHPublicKey)
	}
	if specs.Management.Endpoints.SSH != "127.0.0.1:6123" {
		t.Fatalf("management ssh endpoint = %q", specs.Management.Endpoints.SSH)
	}
	if !specs.SSH.UseGVProxyTunnel {
		t.Fatalf("ssh target should use gvproxy tunnel")
	}
	if specs.GVProxy.HostSSHForwardAddr != "127.0.0.1:6123" {
		t.Fatalf("gvproxy ssh forward = %q", specs.GVProxy.HostSSHForwardAddr)
	}

	managementJSON, err := json.Marshal(specs.Management)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(managementJSON), "PRIVATE-KEY-BODY") || strings.Contains(string(managementJSON), "/tmp/host-key") {
		t.Fatalf("management view leaks private ssh material: %s", managementJSON)
	}

	guestJSON, err := json.Marshal(specs.Guest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(guestJSON), "PRIVATE-KEY-BODY") || strings.Contains(string(guestJSON), "/tmp/host-key") {
		t.Fatalf("guest spec leaks host private ssh material: %s", guestJSON)
	}
}

func TestServiceSpecsCopySlices(t *testing.T) {
	machine := &define.Machine{
		MachineSpec: define.MachineSpec{
			Cmdline: define.Cmdline{
				Args: []string{"before"},
				Envs: []string{"A=B"},
			},
			PodmanInfo: define.PodmanInfo{
				GuestPodmanRunWithEnvs: []string{"P=1"},
			},
		},
	}

	specs := NewServiceSpecs(machine)
	machine.Cmdline.Args[0] = "after"
	machine.Cmdline.Envs[0] = "A=C"
	machine.PodmanInfo.GuestPodmanRunWithEnvs[0] = "P=2"

	if specs.Guest.Cmdline.Args[0] != "before" {
		t.Fatalf("guest args alias machine slice")
	}
	if specs.Guest.Cmdline.Envs[0] != "A=B" {
		t.Fatalf("guest envs alias machine slice")
	}
	if specs.Guest.Podman.GuestPodmanRunWithEnvs[0] != "P=1" {
		t.Fatalf("guest podman envs alias machine slice")
	}
}
