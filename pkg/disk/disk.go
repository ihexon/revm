package disk

import (
	"bytes"
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Manager interface {
	Format(ctx context.Context, blkPath, fsType string) error
	Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error)
	Create(ctx context.Context, blkPath string, sizeInMib uint64) error
	FsCheck(ctx context.Context, blkPath string) error
	NewUUID(ctx context.Context, id string, blkPath string) error
}

type RawDiskManager struct {
	tune2fs string
	blkid   string
	e2fsck  string
	mke2fs  string
}

func NewBlkManager() (*RawDiskManager, error) {
	tune2fs, err := static_resources.GetBuiltinTool(os.TempDir(), "tune2fs")
	if err != nil {
		return nil, err
	}
	blkid, err := static_resources.GetBuiltinTool(os.TempDir(), "blkid")
	if err != nil {
		return nil, err
	}
	e2fsck, err := static_resources.GetBuiltinTool(os.TempDir(), "e2fsck")
	if err != nil {
		return nil, err
	}
	mke2fs, err := static_resources.GetBuiltinTool(os.TempDir(), "mke2fs")
	if err != nil {
		return nil, err
	}

	return &RawDiskManager{
		tune2fs: tune2fs,
		blkid:   blkid,
		e2fsck:  e2fsck,
		mke2fs:  mke2fs,
	}, nil
}

func (b RawDiskManager) Format(ctx context.Context, blkPath, fsType string) error {
	switch fsType {
	case "ext4":
		mke2fs := b.mke2fs
		cmd := exec.CommandContext(ctx, mke2fs, "-t", fsType, "-E", "discard", "-F", blkPath)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
		cmd.Stdin = nil
		return cmd.Run()
	default:
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}
}

func (b RawDiskManager) inspect(ctx context.Context, blkPath string, info string) (string, error) {
	cmd := exec.CommandContext(ctx, b.blkid, "-c", filepath.Join(os.TempDir(), "blkid.cache"), "-s", info, "-o", "value", blkPath)
	cmd.Stderr = os.Stderr

	var result bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = &result

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(result.String()), nil
}

func (b RawDiskManager) Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error) {
	blkPath, err := filepath.Abs(blkPath)
	if err != nil {
		return nil, err
	}

	fsUUID, err := b.inspect(ctx, blkPath, "UUID")
	if err != nil {
		return nil, fmt.Errorf("cannot inspect blk device uuid %q: %w", blkPath, err)
	}

	fsType, err := b.inspect(ctx, blkPath, "TYPE")
	if err != nil {
		return nil, fmt.Errorf("cannot inspect blk device type %q: %w", blkPath, err)
	}

	return &define.BlkDev{
		UUID:    fsUUID,
		FsType:  fsType,
		Path:    blkPath,
		MountTo: fmt.Sprintf("/mnt/%s", fsUUID),
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

func (b RawDiskManager) FsCheck(ctx context.Context, blkPath string) error {
	info, err := b.Inspect(ctx, blkPath)
	if err != nil {
		return err
	}

	if info.FsType != "ext4" {
		return nil
	}

	cmd := exec.CommandContext(ctx, b.e2fsck, "-p", blkPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}

func (b RawDiskManager) NewUUID(ctx context.Context, id string, blkPath string) error {
	blkPath, err := filepath.Abs(blkPath)
	if err != nil {
		return err
	}

	blkPath = filepath.Clean(blkPath)

	cmd := exec.CommandContext(ctx, b.tune2fs, "-U", id, blkPath)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	return cmd.Run()
}
