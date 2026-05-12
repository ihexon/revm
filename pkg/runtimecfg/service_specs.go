//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package runtimecfg

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/protocol"
	"linuxvm/pkg/service/management"
	sshsvc "linuxvm/pkg/service/ssh"
)

type ServiceSpecs struct {
	Guest      protocol.GuestSpec
	Management management.VMConfigView
	SSH        sshsvc.Target
	GVProxy    gvproxy.Spec
}

func NewServiceSpecs(machine *define.Machine) ServiceSpecs {
	return ServiceSpecs{
		Guest:      guestSpecFromMachine(machine),
		Management: managementViewFromMachine(machine),
		SSH:        sshTargetFromMachine(machine),
		GVProxy:    gvproxySpecFromMachine(machine),
	}
}

func guestSpecFromMachine(machine *define.Machine) protocol.GuestSpec {
	if machine == nil {
		return protocol.GuestSpec{SchemaVersion: protocol.GuestSpecVersion}
	}
	return protocol.GuestSpec{
		SchemaVersion: protocol.GuestSpecVersion,
		RunMode:       machine.RunMode,
		NetworkMode:   string(machine.VirtualNetworkMode),
		TTY:           machine.TTY,
		Cmdline:       guestCmdlineFromMachine(machine.Cmdline),
		Mounts:        guestMountsFromMachine(machine.Mounts),
		BlkDevs:       guestBlockDevsFromMachine(machine.BlkDevs),
		SSH:           guestSSHFromMachine(machine.SSHInfo),
		Podman:        guestPodmanFromMachine(machine.PodmanInfo),
	}
}

func guestCmdlineFromMachine(cmd define.Cmdline) protocol.GuestCmdline {
	return protocol.GuestCmdline{
		Envs:    append([]string(nil), cmd.Envs...),
		Bin:     cmd.Bin,
		Args:    append([]string(nil), cmd.Args...),
		WorkDir: cmd.WorkDir,
	}
}

func guestMountsFromMachine(mounts []define.Mount) []protocol.GuestMount {
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

func guestBlockDevsFromMachine(devs []define.BlkDev) []protocol.GuestBlockDev {
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

func guestSSHFromMachine(ssh define.SSHInfo) protocol.GuestSSH {
	return protocol.GuestSSH{
		HostSSHPublicKey:         ssh.HostSSHPublicKey,
		GuestSSHServerListenAddr: ssh.GuestSSHServerListenAddr,
		GuestSSHPrivateKeyFile:   ssh.GuestSSHPrivateKeyFile,
		GuestSSHAuthorizedKeys:   ssh.GuestSSHAuthorizedKeys,
		GuestSSHPidFile:          ssh.GuestSSHPidFile,
	}
}

func guestPodmanFromMachine(podman define.PodmanInfo) protocol.GuestPodman {
	return protocol.GuestPodman{
		GuestPodmanAPIListenAddr: podman.GuestPodmanAPIListenAddr,
		GuestPodmanRunWithEnvs:   append([]string(nil), podman.GuestPodmanRunWithEnvs...),
	}
}

func managementViewFromMachine(machine *define.Machine) management.VMConfigView {
	if machine == nil {
		return management.VMConfigView{}
	}

	view := management.VMConfigView{
		RunMode:     machine.RunMode,
		NetworkMode: string(machine.VirtualNetworkMode),
		Resources: management.ResourceView{
			MemoryInMB: machine.MemoryInMB,
			CPUs:       machine.Cpus,
		},
		Endpoints: management.EndpointView{
			ManagementAPI: machine.VMCtlAddr,
			PodmanAPI:     machine.PodmanInfo.HostPodmanProxyAddr,
			SSH:           machine.SSHInfo.HostSSHProxyListenAddr,
		},
		TTY: machine.TTY,
	}

	for _, mount := range machine.Mounts {
		view.Mounts = append(view.Mounts, management.MountView{
			ReadOnly: mount.ReadOnly,
			Source:   mount.Source,
			Target:   mount.Target,
			Type:     mount.Type,
		})
	}

	for _, disk := range machine.BlkDevs {
		view.Disks = append(view.Disks, management.DiskView{
			UUID:    disk.UUID,
			MountTo: disk.MountTo,
			FsType:  disk.FsType,
		})
	}

	return view
}

func sshTargetFromMachine(machine *define.Machine) sshsvc.Target {
	if machine == nil {
		return sshsvc.Target{User: define.DefaultGuestUser, GuestTunnelHost: define.GuestIP}
	}
	return sshsvc.Target{
		User:                     define.DefaultGuestUser,
		PrivateKeyFile:           machine.SSHInfo.HostSSHPrivateKeyFile,
		UseGVProxyTunnel:         machine.VirtualNetworkMode == define.GVISOR,
		GVPCtlAddr:               machine.GVPCtlAddr,
		GuestSSHServerListenAddr: machine.SSHInfo.GuestSSHServerListenAddr,
		GuestTunnelHost:          define.GuestIP,
	}
}

func gvproxySpecFromMachine(machine *define.Machine) gvproxy.Spec {
	if machine == nil {
		return gvproxy.Spec{}
	}
	return gvproxy.Spec{
		ControlAddr:         machine.GVPCtlAddr,
		NetAddr:             machine.GVPVNetAddr,
		NotifyAddr:          machine.GVPNotifyAddr,
		HostSSHForwardAddr:  machine.SSHInfo.HostSSHProxyListenAddr,
		GuestSSHListenAddr:  machine.SSHInfo.GuestSSHServerListenAddr,
		GuestIP:             define.GuestIP,
		HostLoopbackAddress: define.LocalHost,
	}
}
