//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"unsafe"
)

func (v *Libkrun) setupStorage() error {
	for _, disk := range v.cfg.Storage.Blocks {
		if err := v.addDisk(disk); err != nil {
			return err
		}
	}
	for _, mount := range v.cfg.Storage.VirtioFS {
		if err := v.addVirtioFS(mount); err != nil {
			return err
		}
	}
	return nil
}

func (v *Libkrun) addDisk(spec define.BlockDeviceSpec) error {
	stat, err := os.Stat(spec.Path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() && stat.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("not a regular file or device: %s", spec.Path)
	}
	idValue := spec.ID
	if idValue == "" && spec.GuestMount != nil {
		idValue = spec.GuestMount.UUID
	}
	if idValue == "" {
		idValue = filepath.Base(spec.Path)
	}
	format := spec.Format
	if format == 0 {
		format = define.DiskFormatRaw
	}
	syncMode := spec.SyncMode
	if syncMode == 0 {
		syncMode = define.SyncModeRelaxed
	}
	id := newKrunStr(idValue)
	diskPath := newKrunStr(spec.Path)
	defer id.free()
	defer diskPath.free()
	var errOut C.KrunError
	device := C.krun_block_device_new(id.value, diskPath.value, C.uint32_t(format), &errOut)
	if err := checkKrunHandle("create block device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	C.krun_block_device_set_read_only(device, C.bool(spec.ReadOnly))
	C.krun_block_device_set_direct_io(device, C.bool(spec.DirectIO))
	C.krun_block_device_set_sync_mode(device, C.uint32_t(syncMode))
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}

func (v *Libkrun) addVirtioFS(spec define.VirtioFSSpec) error {
	tag, hostPath, readOnly := spec.Tag, spec.Source, spec.ReadOnly
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
	if err := checkKrunHandle("create virtiofs device", unsafe.Pointer(device), errOut); err != nil {
		return err
	}
	daxWindow := spec.DAXWindowSize
	if daxWindow == 0 {
		daxWindow = virtiofsMemWindow
	}
	C.krun_fs_device_set_dax_window_size(device, C.uint64_t(daxWindow))
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(device)))
	return nil
}
