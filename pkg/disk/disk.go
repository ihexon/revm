package disk

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

type Manager interface {
	Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error)
	Create(ctx context.Context, blkPath string, sizeInMib uint64) error
	NewUUID(ctx context.Context, id string, blkPath string) error
}

type RawDiskManager struct {
}

func NewBlkManager() (*RawDiskManager, error) {
	return &RawDiskManager{}, nil
}

func (b RawDiskManager) Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error) {
	blkPath, err := filepath.Abs(blkPath)
	if err != nil {
		return nil, err
	}

	info, err := ProbeRAWDisk(blkPath)
	if err != nil {
		return nil, err
	}

	return &define.BlkDev{
		UUID:   info.UUID,
		FsType: info.Type,
		Path:   blkPath,
	}, nil
}

func (b RawDiskManager) Create(ctx context.Context, blkPath string, sizeInMib uint64) error {
	f, err := os.OpenFile(blkPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Truncate(int64(filesystem.MiB(sizeInMib).ToBytes()))
}

func (b RawDiskManager) NewUUID(ctx context.Context, id string, blkPath string) error {
	blkPath, err := filepath.Abs(blkPath)
	if err != nil {
		return err
	}

	logrus.Debugf("change uuid of ext4 raw disk %v to %v", blkPath, id)
	_, _, err = SetUUID(blkPath, id)
	if err != nil {
		return err
	}

	return nil
}
