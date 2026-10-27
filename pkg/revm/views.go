//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"linuxvm/pkg/service/guestcontrol"
	"linuxvm/pkg/service/management"
)

func (vm *VM) IgnitionListenAddr() string { return vm.spec.IgnitionServerCfg.ListenSockAddr }

func (vm *VM) GuestSpec() protocol.GuestSpec {
	return protocol.GuestSpec{
		SchemaVersion: protocol.GuestSpecVersion,
		RunMode:       vm.spec.RunMode,
		NetworkMode:   string(vm.spec.VirtualNetworkMode),
		Storage: protocol.GuestStorage{
			VirtioFS: guestMountsFromSpec(vm.spec.Storage.VirtioFS),
			Blocks:   guestBlockDevsFromSpec(vm.spec.Storage.Blocks),
		},
		Podman: guestPodmanFromSpec(vm.spec.PodmanInfo),
	}
}

func (vm *VM) GuestControlTarget() guestcontrol.Target {
	return guestcontrol.Target{CID: define.GuestVSockCID, Port: define.GuestControlPort, UnixSocket: unixSocketPath(vm.spec.GuestControlAddr)}
}

func unixSocketPath(raw string) string {
	addr, err := network.ParseUnixAddr(raw)
	if err != nil {
		return ""
	}
	return addr.Path
}

func (vm *VM) ManagementView() management.VMConfigView {
	view := management.VMConfigView{
		RunMode:     vm.spec.RunMode,
		NetworkMode: string(vm.spec.VirtualNetworkMode),
		Resources: management.ResourceView{
			MemoryInMB: vm.spec.MemoryInMB,
			CPUs:       vm.spec.Cpus,
		},
		Endpoints: management.EndpointView{
			ManagementAPI: vm.spec.VMCtlAddr,
			GVProxyAPI:    vm.spec.GVPCtlAddr,
			PodmanAPI:     vm.spec.PodmanInfo.HostPodmanProxyAddr,
			GuestControl:  vm.spec.GuestControlAddr,
		},
	}

	for _, mount := range vm.spec.Storage.VirtioFS {
		view.Mounts = append(view.Mounts, management.MountView{
			ReadOnly: mount.ReadOnly,
			Source:   mount.Source,
			Target:   mount.Target,
			Type:     mount.Type,
		})
	}

	for _, disk := range vm.spec.Storage.Blocks {
		diskView := management.DiskView{
			ReadOnly: disk.ReadOnly,
			DirectIO: disk.DirectIO,
			SyncMode: disk.SyncMode,
		}
		if disk.GuestMount != nil {
			diskView.UUID = disk.GuestMount.UUID
			diskView.MountTo = disk.GuestMount.Target
			diskView.FsType = disk.GuestMount.FsType
			diskView.ReadOnly = diskView.ReadOnly || disk.GuestMount.ReadOnly
		}
		view.Disks = append(view.Disks, diskView)
	}

	return view
}

func (vm *VM) GVProxySpec() gvproxy.Spec {
	return gvproxy.Spec{
		ControlAddr:         vm.spec.GVPCtlAddr,
		NetAddr:             vm.spec.GVPVNetAddr,
		NotifyAddr:          vm.spec.GVPNotifyAddr,
		GuestIP:             define.GuestIP,
		HostLoopbackAddress: define.LocalHost,
		PortForwards:        append([]define.PortForward(nil), vm.spec.PortForwards...),
	}
}

func guestMountsFromSpec(mounts []define.VirtioFSSpec) []protocol.GuestMount {
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

func guestBlockDevsFromSpec(devs []define.BlockDeviceSpec) []protocol.GuestBlockDev {
	out := make([]protocol.GuestBlockDev, 0, len(devs))
	for _, d := range devs {
		if d.GuestMount == nil {
			continue
		}
		out = append(out, protocol.GuestBlockDev{
			FsType:   d.GuestMount.FsType,
			UUID:     d.GuestMount.UUID,
			Path:     d.Path,
			MountTo:  d.GuestMount.Target,
			ReadOnly: d.ReadOnly || d.GuestMount.ReadOnly,
		})
	}
	return out
}

func guestPodmanFromSpec(podman define.PodmanInfo) protocol.GuestPodman {
	return protocol.GuestPodman{
		GuestPodmanRunWithEnvs: append([]string(nil), podman.GuestPodmanRunWithEnvs...),
	}
}
