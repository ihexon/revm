//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func (v *Libkrun) setupStorage() error {
	for _, disk := range v.cfg.BlkDevs {
		if err := v.addDisk(disk.Path); err != nil {
			return err
		}
	}
	for _, mount := range v.cfg.Mounts {
		if err := v.addVirtioFS(mount.Tag, mount.Source, mount.ReadOnly); err != nil {
			return err
		}
	}
	return nil
}

func (v *Libkrun) addDisk(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	id := newKrunStr(uuid.New().String())
	diskPath := newKrunStr(path)
	defer id.free()
	defer diskPath.free()
	var errOut C.KrunError
	device := C.krun_block_device_new(id.value, diskPath.value, C.KRUN_DISK_FORMAT_RAW, &errOut)
	if err := checkKrunHandle("create block device", device, errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, device)
	return nil
}

func (v *Libkrun) addVirtioFS(tag, hostPath string, readOnly bool) error {
	absPath, err := filepath.Abs(hostPath)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("not a directory: %s", resolved)
	}
	tagC := newKrunStr(tag)
	pathC := newKrunStr(resolved)
	defer tagC.free()
	defer pathC.free()
	var errOut C.KrunError
	var device C.KrunFsDevice
	if readOnly {
		device = C.krun_fs_device_new_read_only(tagC.value, pathC.value, &errOut)
	} else {
		device = C.krun_fs_device_new(tagC.value, pathC.value, &errOut)
	}
	if err := checkKrunHandle("create virtiofs device", device, errOut); err != nil {
		return err
	}
	C.krun_fs_device_set_dax_window_size(device, C.uint64_t(virtiofsMemWindow))
	C.krun_mmio_device_manager_add(v.manager, device)
	return nil
}
