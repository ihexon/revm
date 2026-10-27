//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"linuxvm/pkg/gvproxy"
	"linuxvm/pkg/service/management"
)

func (vm *VM) ManagementView() management.VMConfigView {
	view := management.VMConfigView{
		RunMode:     string(vm.spec.RunMode),
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
			Type:     "virtiofs",
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
