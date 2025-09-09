package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
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

func Fscheck(ctx context.Context, diskFile string) error {
	info, err := GetBlockInfo(ctx, diskFile)
	if err != nil {
		return fmt.Errorf("failed to get disk info: %w", err)
	}

	if info.FilesystemType != "ext4" {
		return fmt.Errorf("filesystem integrity check only support ext4 filesystem, but got %q", info.FilesystemType)
	}

	fsckExt4, err := system.Get3rdUtilsPath("fsck.ext4")
	if err != nil {
		return fmt.Errorf("failed to get mke2fs path: %w", err)
	}

	cmd := exec.CommandContext(ctx, fsckExt4, "-p", info.AbsPath)

	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}
	cmd.Stdin = nil
	logrus.Debugf("fsck cmdline: %q", cmd.Args)

	return cmd.Run()
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
		exists, err := PathExists(path)
		if err != nil {
			return fmt.Errorf("failed to check raw disk %q exists: %w", path, err)
		}

		if exists {
			logrus.Debugf("raw disk %q already exists, skip create and format raw disk", path)
			return nil
		}
	}

	_, err := uuid.Parse(myUUID)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %w", err)
	}

	rawDisk, err := NewDisk(define.DefaultCreateDiskSizeInGB, path, true, define.DiskFormat, myUUID).Create()
	if err != nil {
		return fmt.Errorf("failed to create raw disk: %w", err)
	}

	return rawDisk.Format(ctx)
}

func NewDisk(sizeInGB uint64, path string, shouldFormat bool, filesystemType string, uuid string) *Disk {
	disk := &Disk{
		size:           GiB(sizeInGB),
		AbsPath:        path,
		shouldFormat:   shouldFormat,
		FilesystemType: filesystemType,
		UUID:           uuid,
	}

	buffer, _ := json.Marshal(disk)
	logrus.Debugf("the structure of raw disk: %q", string(buffer))
	return disk
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

	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}
	cmd.Stdin = nil

	logrus.Debugf("mke2fs cmdline: %q", cmd.Args)
	logrus.Infof("format disk %q with filesystem %q UUID: %q", d.AbsPath, d.FilesystemType, d.UUID)
	return cmd.Run()
}

func (d *Disk) Create() (*Disk, error) {
	logrus.Infof("create disk %q", d.AbsPath)
	f, err := os.OpenFile(d.AbsPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	logrus.Infof("truncate disk %q to %d GB", d.AbsPath, d.size)
	return d, f.Truncate(int64(d.size.ToBytes()))
}
