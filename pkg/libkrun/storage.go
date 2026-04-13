//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <libkrun.h>
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const virtiofsMemWindow = 512 << 20 // 512MB

// setupStorage configures block devices and virtiofs mounts.
func (v *Libkrun) setupStorage() error {
	for _, disk := range v.cfg.BlkDevs {
		if err := v.addDisk(disk.Path); err != nil {
			return err
		}
	}

	for _, mount := range v.cfg.Mounts {
		if err := v.addVirtioFS(mount.Tag, mount.Source); err != nil {
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

	id := cstr(uuid.New().String())
	defer free(id)

	diskPath := cstr(path)
	defer free(diskPath)

	ret := C.krun_add_disk2(
		C.uint32_t(v.ctxID),
		id,
		diskPath,
		C.KRUN_DISK_FORMAT_RAW,
		false,
	)
	if ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (v *Libkrun) addVirtioFS(tag, hostPath string) error {
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

	tagC := cstr(tag)
	defer free(tagC)

	pathC := cstr(resolved)
	defer free(pathC)

	ret := C.krun_add_virtiofs2(
		C.uint32_t(v.ctxID),
		tagC,
		pathC,
		C.uint64_t(virtiofsMemWindow),
	)
	if ret != 0 {
		return errCode(ret)
	}
	return nil
}
