package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/system"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

type Disk struct {
	size           StorageUnits
	path           string
	shouldFormat   bool
	FilesystemType string
	UUID           string
}

func GetBlockInfo(ctx context.Context, block string) (*Disk, error) {
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
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk UUID: %w", err)
	}
	logrus.Infof("blkid UUID output: %q", strings.TrimSpace(uuid.String()))

	cmd = exec.CommandContext(ctx, blkid, "-s", "TYPE", "-o", "value", block)
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	var fsType bytes.Buffer
	cmd.Stdout = &fsType
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get disk filesystem type: %w", err)
	}
	logrus.Infof("blkid fs type output: %q", strings.TrimSpace(fsType.String()))

	DataDiskInfo := Disk{
		UUID:           strings.TrimSpace(uuid.String()),
		FilesystemType: strings.TrimSpace(fsType.String()),
		path:           block,
	}

	return &DataDiskInfo, nil
}

func CreateDiskAndFormatExt4(ctx context.Context, path string, overwrite bool) error {
	if !overwrite {
		if exists, _ := PathExists(path); exists {
			logrus.Infof("%q disk already exists, skip", path)
			return nil
		}
	}

	rawDisk, err := NewDisk(define.DiskSizeInGB, path, true, define.DiskFormat, define.DiskUUID).Create()
	if err != nil {
		return fmt.Errorf("failed to create raw disk: %w", err)
	}

	return rawDisk.Format(ctx)
}

func NewDisk(sizeInGB uint64, path string, shouldFormat bool, filesystemType string, uuid string) *Disk {
	return &Disk{
		size:           GiB(sizeInGB),
		path:           path,
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
	cmd.Args = append(cmd.Args, d.path)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Stdin = nil
	logrus.Infof("cmdline: %q", cmd.Args)
	return cmd.Run()
}

func (d *Disk) Create() (*Disk, error) {
	f, err := os.OpenFile(d.path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
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
