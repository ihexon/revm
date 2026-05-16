//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package machine

import (
	"context"
	"fmt"

	"linuxvm/pkg/backend"
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/protocol"
	"linuxvm/pkg/service/management"
	sshsvc "linuxvm/pkg/service/ssh"
)

// Machine is the internal runtime view of a VM.
// Public protocols and service views are derived from this type instead of
// being threaded through service constructors as separate arguments.
type Machine struct {
	spec    *define.MachineSpec
	backend backend.Backend
}

func New(spec *define.MachineSpec, backend backend.Backend) (*Machine, error) {
	if spec == nil {
		return nil, fmt.Errorf("machine spec is nil")
	}
	if backend == nil {
		return nil, fmt.Errorf("backend is nil")
	}
	return &Machine{spec: spec, backend: backend}, nil
}

func (m *Machine) Start(vmWaitAbortCtx context.Context) error {
	// Preserve the vmWaitAbortCtx name while forwarding it so callers can distinguish
	// aborting the host-side VM wait from requesting guest shutdown.
	return m.backend.Start(vmWaitAbortCtx)
}

func (m *Machine) Stop() error {
	return m.backend.Stop()
}

func (m *Machine) RunMode() string {
	return m.spec.RunMode
}

func (m *Machine) VirtualNetworkMode() define.VNetMode {
	return m.spec.VirtualNetworkMode
}

func (m *Machine) PodmanHostProxyAddr() string {
	return m.spec.PodmanInfo.HostPodmanProxyAddr
}

func (m *Machine) PodmanGuestAPIListenAddr() string {
	return m.spec.PodmanInfo.GuestPodmanAPIListenAddr
}

func (m *Machine) GVPCtlAddr() string {
	return m.spec.GVPCtlAddr
}

func (m *Machine) IgnitionListenAddr() string {
	return m.spec.IgnitionServerCfg.ListenSockAddr
}

func (m *Machine) ManagementAPIEndpoint() string {
	return m.spec.VMCtlAddr
}

func (m *Machine) GuestSpec() protocol.GuestSpec {
	return protocol.GuestSpec{
		SchemaVersion: protocol.GuestSpecVersion,
		RunMode:       m.spec.RunMode,
		NetworkMode:   string(m.spec.VirtualNetworkMode),
		TTY:           m.spec.TTY,
		Cmdline:       guestCmdlineFromSpec(m.spec.Cmdline),
		Mounts:        guestMountsFromSpec(m.spec.Mounts),
		BlkDevs:       guestBlockDevsFromSpec(m.spec.BlkDevs),
		SSH:           guestSSHFromSpec(m.spec.SSHInfo),
		Podman:        guestPodmanFromSpec(m.spec.PodmanInfo),
	}
}

func (m *Machine) AttachSpec() protocol.AttachSpec {
	sshTarget := m.SSHTarget()
	return protocol.AttachSpec{
		SchemaVersion:            protocol.AttachSpecVersion,
		User:                     sshTarget.User,
		PrivateKeyFile:           sshTarget.PrivateKeyFile,
		UseGVProxyTunnel:         sshTarget.UseGVProxyTunnel,
		GVPCtlAddr:               sshTarget.GVPCtlAddr,
		GuestSSHServerListenAddr: sshTarget.GuestSSHServerListenAddr,
		GuestTunnelHost:          sshTarget.GuestTunnelHost,
	}
}

func (m *Machine) ManagementView() management.VMConfigView {
	view := management.VMConfigView{
		RunMode:     m.spec.RunMode,
		NetworkMode: string(m.spec.VirtualNetworkMode),
		Resources: management.ResourceView{
			MemoryInMB: m.spec.MemoryInMB,
			CPUs:       m.spec.Cpus,
		},
		Endpoints: management.EndpointView{
			ManagementAPI: m.spec.VMCtlAddr,
			PodmanAPI:     m.spec.PodmanInfo.HostPodmanProxyAddr,
			SSH:           m.spec.SSHInfo.HostSSHProxyListenAddr,
		},
		TTY: m.spec.TTY,
	}

	for _, mount := range m.spec.Mounts {
		view.Mounts = append(view.Mounts, management.MountView{
			ReadOnly: mount.ReadOnly,
			Source:   mount.Source,
			Target:   mount.Target,
			Type:     mount.Type,
		})
	}

	for _, disk := range m.spec.BlkDevs {
		view.Disks = append(view.Disks, management.DiskView{
			UUID:    disk.UUID,
			MountTo: disk.MountTo,
			FsType:  disk.FsType,
		})
	}

	return view
}

func (m *Machine) SSHTarget() sshsvc.Target {
	return sshsvc.Target{
		User:                     define.DefaultGuestUser,
		PrivateKeyFile:           m.spec.SSHInfo.HostSSHPrivateKeyFile,
		UseGVProxyTunnel:         m.spec.VirtualNetworkMode == define.GVISOR,
		GVPCtlAddr:               m.spec.GVPCtlAddr,
		GuestSSHServerListenAddr: m.spec.SSHInfo.GuestSSHServerListenAddr,
		GuestTunnelHost:          define.GuestIP,
	}
}

func (m *Machine) GVProxySpec() gvproxy.Spec {
	return gvproxy.Spec{
		ControlAddr:         m.spec.GVPCtlAddr,
		NetAddr:             m.spec.GVPVNetAddr,
		NotifyAddr:          m.spec.GVPNotifyAddr,
		HostSSHForwardAddr:  m.spec.SSHInfo.HostSSHProxyListenAddr,
		GuestSSHListenAddr:  m.spec.SSHInfo.GuestSSHServerListenAddr,
		GuestIP:             define.GuestIP,
		HostLoopbackAddress: define.LocalHost,
	}
}

func guestCmdlineFromSpec(cmd define.Cmdline) protocol.GuestCmdline {
	return protocol.GuestCmdline{
		Envs:    append([]string(nil), cmd.Envs...),
		Bin:     cmd.Bin,
		Args:    append([]string(nil), cmd.Args...),
		WorkDir: cmd.WorkDir,
	}
}

func guestMountsFromSpec(mounts []define.Mount) []protocol.GuestMount {
	out := make([]protocol.GuestMount, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, protocol.GuestMount{
			ReadOnly: m.ReadOnly,
			Source:   m.Source,
			Tag:      m.Tag,
			Target:   m.Target,
			Type:     m.Type,
			Opts:     m.Opts,
			UUID:     m.UUID,
		})
	}
	return out
}

func guestBlockDevsFromSpec(devs []define.BlkDev) []protocol.GuestBlockDev {
	out := make([]protocol.GuestBlockDev, 0, len(devs))
	for _, d := range devs {
		out = append(out, protocol.GuestBlockDev{
			FsType:  d.FsType,
			UUID:    d.UUID,
			Path:    d.Path,
			MountTo: d.MountTo,
		})
	}
	return out
}

func guestSSHFromSpec(ssh define.SSHInfo) protocol.GuestSSH {
	return protocol.GuestSSH{
		HostSSHPublicKey:         ssh.HostSSHPublicKey,
		GuestSSHServerListenAddr: ssh.GuestSSHServerListenAddr,
		GuestSSHPrivateKeyFile:   ssh.GuestSSHPrivateKeyFile,
		GuestSSHAuthorizedKeys:   ssh.GuestSSHAuthorizedKeys,
		GuestSSHPidFile:          ssh.GuestSSHPidFile,
	}
}

func guestPodmanFromSpec(podman define.PodmanInfo) protocol.GuestPodman {
	return protocol.GuestPodman{
		GuestPodmanAPIListenAddr: podman.GuestPodmanAPIListenAddr,
		GuestPodmanRunWithEnvs:   append([]string(nil), podman.GuestPodmanRunWithEnvs...),
	}
}
