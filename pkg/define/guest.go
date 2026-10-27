package define

import "linuxvm/pkg/protocol"

func (spec *MachineSpec) GuestSpec() protocol.GuestSpec {
	return protocol.GuestSpec{
		SchemaVersion: protocol.GuestSpecVersion,
		RunMode:       spec.RunMode,
		NetworkMode:   string(spec.VirtualNetworkMode),
		Storage: protocol.GuestStorage{
			VirtioFS: guestMountsFromSpec(spec.Storage.VirtioFS),
			Blocks:   guestBlockDevsFromSpec(spec.Storage.Blocks),
		},
		Podman: guestPodmanFromSpec(spec.PodmanInfo),
	}
}

func guestMountsFromSpec(mounts []VirtioFSSpec) []protocol.GuestMount {
	out := make([]protocol.GuestMount, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, protocol.GuestMount{
			ReadOnly: m.ReadOnly,
			Tag:      m.Tag,
			Target:   m.Target,
		})
	}
	return out
}

func guestBlockDevsFromSpec(devs []BlockDeviceSpec) []protocol.GuestBlockDev {
	out := make([]protocol.GuestBlockDev, 0, len(devs))
	for _, d := range devs {
		if d.GuestMount == nil {
			continue
		}
		out = append(out, protocol.GuestBlockDev{
			FsType:   d.GuestMount.FsType,
			UUID:     d.GuestMount.UUID,
			MountTo:  d.GuestMount.Target,
			ReadOnly: d.ReadOnly || d.GuestMount.ReadOnly,
		})
	}
	return out
}

func guestPodmanFromSpec(podman PodmanInfo) protocol.GuestPodman {
	return protocol.GuestPodman{
		GuestPodmanRunWithEnvs: append([]string(nil), podman.GuestPodmanRunWithEnvs...),
	}
}
