package disk

import (
	"bytes"
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/filesystem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

type Manager interface {
	Format(ctx context.Context, blkPath, fsType string) error
	Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error)
	Create(ctx context.Context, blkPath string, sizeInMib uint64) error
	FsCheck(ctx context.Context, blkPath string) error
}

type BlkManager struct {
	tools struct {
		mkfsExt4 string
		blkid    string
		fsckExt4 string
	}
}

func NewBlkManager(mkfsExt4, blkid, fsckExt4 string) *BlkManager {
	return &BlkManager{
		tools: struct {
			mkfsExt4 string
			blkid    string
			fsckExt4 string
		}{
			mkfsExt4: mkfsExt4,
			blkid:    blkid,
			fsckExt4: fsckExt4,
		},
	}
}

func (b BlkManager) Format(ctx context.Context, blkPath, fsType string) error {
	switch fsType {
	case "ext4":
		mke2fs := b.tools.mkfsExt4
		cmd := exec.CommandContext(ctx, mke2fs, "-t", fsType, "-E", "discard", "-F", blkPath)
		if logrus.GetLevel() == logrus.DebugLevel {
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stderr
		}
		cmd.Stdin = nil

		logrus.Debugf("mke2fs cmdline: %q", cmd.Args)
		return cmd.Run()
	default:
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}
}

func (b BlkManager) inspect(ctx context.Context, blkPath string, info string) (string, error) {
	cmd := exec.CommandContext(ctx, b.tools.blkid, "-c", filepath.Join(os.TempDir(), "blkid.cache"), "-s", info, "-o", "value", blkPath)
	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
	}
	var result bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = &result

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(result.String()), nil
}

func (b BlkManager) Inspect(ctx context.Context, blkPath string) (*define.BlkDev, error) {
	fsUUID, err := b.inspect(ctx, blkPath, "UUID")
	if err != nil {
		return nil, err
	}

	fsType, err := b.inspect(ctx, blkPath, "TYPE")
	if err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(blkPath)
	if err != nil {
		return nil, err
	}

	return &define.BlkDev{
		UUID:   fsUUID,
		FsType: fsType,
		Path:   abs,
	}, nil
}

func (b BlkManager) Create(ctx context.Context, blkPath string, sizeInMib uint64) error {
	f, err := os.OpenFile(blkPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	defer func(f *os.File) {
		if err := f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	return f.Truncate(int64(filesystem.MiB(sizeInMib).ToBytes()))
}

func (b BlkManager) FsCheck(ctx context.Context, blkPath string) error {
	info, err := b.Inspect(ctx, blkPath)
	if err != nil {
		return err
	}

	if info.FsType != "ext4" {
		logrus.Warnf("filesystem integrity check only support ext4 filesystem")
		return nil
	}

	cmd := exec.CommandContext(ctx, b.tools.fsckExt4, "-p", blkPath)
	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}
	cmd.Stdin = nil

	logrus.Debugf("fsck.ext4 cmdline: %q", cmd.Args)
	return cmd.Run()
}
