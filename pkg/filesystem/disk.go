package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/path"
	"linuxvm/pkg/system"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type DiskInfo struct {
	UUID           string
	FilesystemType string
	AbsPath        string
}

func getBlockInfo(ctx context.Context, filePath string) (*RawDisk, error) {
	block, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	blkid, err := path.GetDarwinToolsPath("blkid")
	if err != nil {
		return nil, fmt.Errorf("failed to get blkid path: %w", err)
	}

	// blkid -s UUID -o value /dev/sda1
	cmd := exec.CommandContext(ctx, blkid, "-c", "/dev/null", "-s", "UUID", "-o", "value", block)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	var uuid bytes.Buffer
	cmd.Stdout = &uuid

	logrus.Debugf("blkid UUID cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk UUID: %w", err)
	}

	logrus.Debugf("blkid UUID output: %q", strings.TrimSpace(uuid.String()))
	cmd = exec.CommandContext(ctx, blkid, "-c", "/dev/null", "-s", "TYPE", "-o", "value", block)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	var fsType bytes.Buffer
	cmd.Stdout = &fsType
	logrus.Debugf("blkid fs type cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk filesystem type: %w", err)
	}
	logrus.Debugf("blkid fs type output: %q", strings.TrimSpace(fsType.String()))

	DataDiskInfo := RawDisk{
		UUID:   strings.TrimSpace(uuid.String()),
		FsType: strings.TrimSpace(fsType.String()),
		Path:   block,
	}

	return &DataDiskInfo, nil
}

func NewDisk(path string) RawDisk {
	disk := RawDisk{
		SizeInGB: define.DefaultCreateDiskSizeInGB,
		Path:     path,
	}

	return disk
}

type RawDisk define.DataDisk

func (disk *RawDisk) FsCheck(ctx context.Context) error {
	info, err := getBlockInfo(ctx, disk.Path)
	if err != nil {
		return fmt.Errorf("get disk %q info error: %w", disk.Path, err)
	}

	if info.FsType != "ext4" {
		return fmt.Errorf("filesystem integrity check only support ext4 filesystem, but got %q", info.FsType)
	}

	fsckExt4, err := path.GetDarwinToolsPath("fsck.ext4")
	if err != nil {
		return fmt.Errorf("failed to get fsck.ext4 path: %w", err)
	}

	cmd := exec.CommandContext(ctx, fsckExt4, "-p", disk.Path)

	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}

	cmd.Stdin = nil

	logrus.Debugf("fsck cmdline: %q", cmd.Args)

	return cmd.Run()
}

func (disk *RawDisk) SetUUID(uuid string) {
	disk.UUID = uuid
}

func (disk *RawDisk) SetFileSystemType(fsType string) {
	disk.FsType = fsType
}

func (disk *RawDisk) SetSizeInGB(sizeInGB uint64) {
	disk.SizeInGB = sizeInGB
}

func (disk *RawDisk) Format(ctx context.Context) error {
	mke2fs, err := path.GetDarwinToolsPath("mke2fs")
	if err != nil {
		return fmt.Errorf("failed to get mke2fs path: %w", err)
	}

	if disk.FsType == "" {
		return fmt.Errorf("filesystem type is empty")
	}

	cmd := exec.CommandContext(ctx, mke2fs, "-t", disk.FsType, "-E", "discard", "-F")

	if disk.UUID != "" {
		cmd.Args = append(cmd.Args, "-U", disk.UUID)
	}

	cmd.Args = append(cmd.Args, disk.Path)

	if logrus.GetLevel() == logrus.DebugLevel {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}
	cmd.Stdin = nil

	logrus.Debugf("mke2fs cmdline: %q", cmd.Args)
	return cmd.Run()
}

func (disk *RawDisk) Create() error {
	f, err := os.OpenFile(disk.Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	defer func(f *os.File) {
		if err := f.Close(); err != nil {
			logrus.Errorf("failed to close file: %v", err)
		}
	}(f)

	logrus.Infof("truncate disk %q to %d GB", disk.Path, disk.SizeInGB)

	return f.Truncate(int64(GiB(disk.SizeInGB).ToBytes()))
}

func (disk *RawDisk) Inspect(ctx context.Context) error {
	info, err := getBlockInfo(ctx, disk.Path)
	if err != nil {
		return fmt.Errorf("failed to get block %q info: %w", disk.Path, err)
	}
	disk.UUID = info.UUID
	disk.FsType = info.FsType
	disk.Path = info.Path

	disk.MountTo = filepath.Join(define.DefaultDataDiskMountDirPrefix, info.Path)
	if disk.IsContainerStorage {
		disk.MountTo = define.ContainerStorageMountPoint
	}

	return nil
}

func (disk *RawDisk) GetFileSystemType() string {
	return disk.FsType
}

func (disk *RawDisk) CreateExt4DiskAndFormat(ctx context.Context) error {
	var err error
	cleanup := system.CleanUp()
	defer cleanup.CleanIfErr(&err)

	disk.SetFileSystemType(define.Ext4)
	disk.SetUUID(uuid.New().String())
	disk.SetSizeInGB(define.DefaultCreateDiskSizeInGB)

	cleanup.Add(func() error {
		return os.Remove(disk.Path)
	})

	if err = disk.Create(); err != nil {
		return err
	}

	if err = disk.Format(ctx); err != nil {
		return err
	}

	return nil
}
