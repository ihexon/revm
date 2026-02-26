//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package vmbuilder

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func (v *VM) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	if err = static_resources.ExtractEmbeddedRawDisk(ctx, rawDiskPath); err != nil {
		return fmt.Errorf("failed to extract embedded raw disk: %w", err)
	}

	if err = diskMgr.NewUUID(ctx, givenUUID, rawDiskPath); err != nil {
		return fmt.Errorf("failed to write UUID for raw disk %q: %w", rawDiskPath, err)
	}

	xattrWriter := filesystem.NewXATTRManager()

	for xattrKind, xattrValue := range v.XATTRSRawDisk {
		if err = xattrWriter.WriteXATTR(ctx, rawDiskPath, xattrKind, xattrValue, true); err != nil {
			return fmt.Errorf("failed to write xattr %s to %s: %w", xattrKind, rawDiskPath, err)
		}
	}

	return nil
}

func (v *VM) configureContainerRAWDisk(ctx context.Context, pathMgr *PathManager) error {
	rawDiskFilePath := pathMgr.GetContainerStorageDiskPath()
	if _, err := os.Stat(rawDiskFilePath); err != nil {
		if err = v.generateRAWDisk(ctx, rawDiskFilePath, define.ContainerDiskUUID); err != nil {
			return err
		}
	}

	return v.addRAWDiskToBlkList(ctx, rawDiskFilePath)
}

func (v *VM) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return err
	}

	if info.UUID == define.ContainerDiskUUID {
		info.MountTo = define.ContainerStorageMountPoint
	}

	blkDev := define.BlkDev{
		UUID:    info.UUID,
		FsType:  info.FsType,
		Path:    info.Path,
		MountTo: info.MountTo,
	}

	v.BlkDevs = append(v.BlkDevs, blkDev)

	return nil
}

func (v *VM) withUserProvidedStorageRAWDisk(ctx context.Context, rawDiskS []string) error {
	for _, f := range rawDiskS {
		if f == "" {
			return fmt.Errorf("raw disk path is empty")
		}

		rawDiskPath, err := filepath.Abs(filepath.Clean(f))
		if err != nil {
			return err
		}
		if _, err = os.Stat(rawDiskPath); err != nil {
			if err = v.generateRAWDisk(ctx, rawDiskPath, uuid.NewString()); err != nil {
				return err
			}
		}

		if err = v.addRAWDiskToBlkList(ctx, rawDiskPath); err != nil {
			return err
		}
	}

	return nil
}

func (v *VM) resetOrReuseContainerRAWDisk(ctx context.Context, pathMgr *PathManager, containerDiskVersionXATTR string) error {
	resetBool, err := v.withRAWDiskVersionXATTR(containerDiskVersionXATTR).needsDiskRegeneration(ctx, pathMgr)
	if err != nil {
		return fmt.Errorf("failed to check RAW disk needs to regenerate: %w", err)
	}

	if resetBool {
		if err := os.Remove(pathMgr.GetContainerStorageDiskPath()); err != nil && !os.IsNotExist(err) {
			return err
		}

		if err := v.configureContainerRAWDisk(ctx, pathMgr); err != nil {
			return fmt.Errorf("failed to attach container storage raw disk: %w", err)
		}
	}

	return nil
}

func (v *VM) needsDiskRegeneration(ctx context.Context, pathMgr *PathManager) (bool, error) {
	xattrKey := define.XATTRRawDiskVersionKey
	xattrProcesser := filesystem.NewXATTRManager()

	value1, _ := xattrProcesser.GetXATTR(ctx, pathMgr.GetContainerStorageDiskPath(), xattrKey)
	value2 := v.XATTRSRawDisk[xattrKey]
	if value2 == "" {
		return false, fmt.Errorf("vmc XATTRSRawDisk not set")
	}

	if value1 != value2 {
		return true, nil
	}

	return false, nil
}

func (v *VM) withRAWDiskVersionXATTR(value string) *VM {
	v.XATTRSRawDisk = map[string]string{
		define.XATTRRawDiskVersionKey: value,
	}
	return v
}
