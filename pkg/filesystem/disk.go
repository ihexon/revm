package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type Disk struct {
	size           StorageUnits
	shouldFormat   bool
	FilesystemType string
	UUID           string
	AbsPath        string
}

func GetBlockInfo(ctx context.Context, path string) (*Disk, error) {
	block, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	blkid, err := system.Get3rdUtilsPath("blkid")
	if err != nil {
		return nil, fmt.Errorf("failed to get blkid path: %w", err)
	}
	// blkid -s UUID -o value /dev/sda1
	cmd := exec.CommandContext(ctx, blkid, "-s", "UUID", "-o", "value", block)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	var uuid bytes.Buffer
	cmd.Stdout = &uuid
	logrus.Debugf("blkid UUID cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk UUID: %w", err)
	}

	logrus.Debugf("blkid UUID output: %q", strings.TrimSpace(uuid.String()))
	cmd = exec.CommandContext(ctx, blkid, "-s", "TYPE", "-o", "value", block)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	var fsType bytes.Buffer
	cmd.Stdout = &fsType
	logrus.Debugf("blkid fs type cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk filesystem type: %w", err)
	}
	logrus.Debugf("blkid fs type output: %q", strings.TrimSpace(fsType.String()))

	DataDiskInfo := Disk{
		UUID:           strings.TrimSpace(uuid.String()),
		FilesystemType: strings.TrimSpace(fsType.String()),
		AbsPath:        block,
	}

	return &DataDiskInfo, nil
}

// CreateDiskAndFormatExt4 creates a raw disk at the specified path and formats it with the ext4 filesystem.
// If the overwrite flag is false and the disk already exists, it skips creation and formatting.
// It also allows specifying a custom UUID or generates a new one if none is provided, if myUUID is empty, it generates a new one.
func CreateDiskAndFormatExt4(ctx context.Context, path string, myUUID string, overwrite bool) error {
	if !overwrite {
		if exists, _ := PathExists(path); exists {
			logrus.Infof("%q disk already exists, skip create and format raw disk", path)
			return nil
		}
	}

	if myUUID == "" {
		myUUID = uuid.NewString()
	}

	logrus.Infof("create raw disk at %q with UUID %q", path, myUUID)
	rawDisk, err := NewDisk(define.DefaultCreateDiskSizeInGB, path, true, define.DiskFormat, myUUID).Create()
	if err != nil {
		return fmt.Errorf("failed to create raw disk: %w", err)
	}

	return rawDisk.Format(ctx)
}

func NewDisk(sizeInGB uint64, path string, shouldFormat bool, filesystemType string, uuid string) *Disk {
	return &Disk{
		size:           GiB(sizeInGB),
		AbsPath:        path,
		shouldFormat:   shouldFormat,
		FilesystemType: filesystemType,
		UUID:           uuid,
	}
}

func (d *Disk) Format(ctx context.Context) error {
	mke2fs, err := system.Get3rdUtilsPath("mke2fs")
	if err != nil {
		return fmt.Errorf("failed to get mke2fs path: %w", err)
	}

	cmd := exec.CommandContext(ctx, mke2fs, "-t", d.FilesystemType, "-E", "discard", "-F")
	if d.UUID != "" {
		cmd.Args = append(cmd.Args, "-U", d.UUID)
	}
	cmd.Args = append(cmd.Args, d.AbsPath)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Stdin = nil
	logrus.Infof("mke2fs cmdline: %q", cmd.Args)
	return cmd.Run()
}

func (d *Disk) Create() (*Disk, error) {
	f, err := os.OpenFile(d.AbsPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	if err = f.Truncate(int64(d.size.ToBytes())); err != nil {
		return nil, fmt.Errorf("failed to truncate file: %w", err)
	}

	return d, nil
}
