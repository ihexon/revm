package static_resources

import (
	"context"
	_ "embed"
	"fmt"

	libarchive_go "linuxvm/pkg/libarchive"
)

//go:embed raw_disks/ext4.raw.tar
var BuiltinRawDiskBytes []byte

//go:embed rootfs/rootfs.tar.zst
var RootfsBytes []byte

//go:embed guest_agent/guest-agent
var GuestAgentBytes []byte

func ExtractEmbeddedRawDisk(ctx context.Context, targetPath string) error {
	return libarchive_go.ExtractDisk(ctx, BuiltinRawDiskBytes, targetPath)
}

func ExtractBuiltinRootfs(ctx context.Context, dstDir string) error {
	return libarchive_go.ExtractRootFS(ctx, RootfsBytes, dstDir)
}

func GuestAgent() ([]byte, error) {
	if len(GuestAgentBytes) < 4 ||
		GuestAgentBytes[0] != 0x7f ||
		GuestAgentBytes[1] != 'E' ||
		GuestAgentBytes[2] != 'L' ||
		GuestAgentBytes[3] != 'F' {
		return nil, fmt.Errorf("embedded guest-agent is missing or invalid; build with scripts/build.go")
	}
	return GuestAgentBytes, nil
}
