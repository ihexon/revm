package disk

import (
	"linuxvm/pkg/define"
	"path/filepath"
)

func Inspect(blkPath string) (*define.BlockDeviceSpec, error) {
	blkPath, err := filepath.Abs(blkPath)
	if err != nil {
		return nil, err
	}

	info, err := probe(blkPath)
	if err != nil {
		return nil, err
	}

	return &define.BlockDeviceSpec{
		ID:       info.UUID,
		Path:     blkPath,
		Format:   define.DiskFormatRaw,
		SyncMode: define.SyncModeRelaxed,
		GuestMount: &define.BlockMountSpec{
			UUID:   info.UUID,
			FsType: info.Type,
		},
	}, nil
}
