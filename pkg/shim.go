package pkg

import (
	"context"
	"linuxvm/pkg/vmconfig"
)

type VMProvider interface {
	CreateVM(ctx context.Context, opts vmconfig.VMConfig)
	StartVM(ctx context.Context, vmc vmconfig.VMConfig)
	StopVM(ctx context.Context, vmc vmconfig.VMConfig)
	MountHostDir2VM(ctx context.Context, vmc vmconfig.VMConfig)
	StartNetworkProvider(ctx context.Context, vmc vmconfig.VMConfig)

	// InspectRootfs Opens the rootfs directory in the default file manager for each operating system
	// (Explorer on Windows, Finder on macOS, or xdg-open on Linux)
	InspectRootfs(ctx context.Context, vmc vmconfig.VMConfig)
}
