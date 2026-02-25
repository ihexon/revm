package disk

/*
#cgo CFLAGS: -I ../../out/.deps/e2fsprogs/include
#cgo LDFLAGS: -L ../../out/.deps/e2fsprogs/lib -lext2fs -lcom_err -le2p -luuid -lblkid -lpthread

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <ext2fs/ext2fs.h>
#include <blkid/blkid.h>

// Probe result structure with flattened fields
typedef struct {
    int error_code;
    char error_msg[256];
    char devname[256];
    // Filesystem info
    char uuid[64];
    char fs_type[64];
    char label[256];
    char version[64];
    // Partition info
    char part_uuid[64];
    char part_label[256];
    // Mount info
    int mount_flags;
    char mount_point[256];
} blkid_probe_result;

// Probe a single device and get all info
blkid_probe_result blkid_probe_device(const char* device) {
    blkid_probe_result result = {0};
    blkid_cache cache = NULL;
    blkid_dev dev = NULL;
    blkid_tag_iterate iter;
    const char *type, *value;

    // Check device exists
    if (access(device, F_OK) != 0) {
        result.error_code = 1;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Device not found: %s", device);
        return result;
    }

    strncpy(result.devname, device, sizeof(result.devname) - 1);

    // Get cache (use /dev/null to disable caching)
    if (blkid_get_cache(&cache, "/dev/null") < 0) {
        result.error_code = 2;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Failed to initialize blkid");
        return result;
    }

    // Probe the device
    dev = blkid_get_dev(cache, device, BLKID_DEV_NORMAL);
    if (!dev) {
        result.error_code = 3;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Failed to probe device (unknown filesystem): %s", device);
        blkid_put_cache(cache);
        return result;
    }

    // Iterate through all tags and extract known fields
    iter = blkid_tag_iterate_begin(dev);
    while (blkid_tag_next(iter, &type, &value) == 0) {
        if (strcmp(type, "UUID") == 0) {
            strncpy(result.uuid, value, sizeof(result.uuid) - 1);
        } else if (strcmp(type, "TYPE") == 0) {
            strncpy(result.fs_type, value, sizeof(result.fs_type) - 1);
        } else if (strcmp(type, "LABEL") == 0) {
            strncpy(result.label, value, sizeof(result.label) - 1);
        } else if (strcmp(type, "VERSION") == 0) {
            strncpy(result.version, value, sizeof(result.version) - 1);
        } else if (strcmp(type, "PARTUUID") == 0) {
            strncpy(result.part_uuid, value, sizeof(result.part_uuid) - 1);
        } else if (strcmp(type, "PARTLABEL") == 0) {
            strncpy(result.part_label, value, sizeof(result.part_label) - 1);
        }
    }
    blkid_tag_iterate_end(iter);

    // Check mount status
    int mount_flags = 0;
    char mount_point[256] = {0};
    if (ext2fs_check_mount_point(device, &mount_flags, mount_point, sizeof(mount_point)) == 0) {
        result.mount_flags = mount_flags;
        strncpy(result.mount_point, mount_point, sizeof(result.mount_point) - 1);
    }

    blkid_put_cache(cache);
    result.error_code = 0;
    return result;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Mount flags
const (
	MountFlagMounted  = 0x0001
	MountFlagReadOnly = 0x0002
	MountFlagBusy     = 0x0008
)

// ProbeResult contains all information from probing a device
type ProbeResult struct {
	Device string

	// Filesystem info
	UUID    string
	Type    string // filesystem type (ext4, xfs, vfat, etc.)
	Label   string
	Version string // filesystem version

	// Partition info (GPT/MBR)
	PartUUID  string
	PartLabel string

	// Mount status
	MountFlags int
	MountPoint string
}

// IsMounted returns true if the device is mounted
func (p *ProbeResult) IsMounted() bool {
	return p.MountFlags&MountFlagMounted != 0
}

// IsReadOnly returns true if the device is mounted read-only
func (p *ProbeResult) IsReadOnly() bool {
	return p.MountFlags&MountFlagReadOnly != 0
}

// IsBusy returns true if the device is busy
func (p *ProbeResult) IsBusy() bool {
	return p.MountFlags&MountFlagBusy != 0
}

// ProbeRAWDisk probes a device and returns filesystem information
func ProbeRAWDisk(device string) (*ProbeResult, error) {
	cDevice := C.CString(device)
	defer C.free(unsafe.Pointer(cDevice))

	result := C.blkid_probe_device(cDevice)
	if result.error_code != 0 {
		return nil, errors.New(C.GoString(&result.error_msg[0]))
	}

	return &ProbeResult{
		Device:     C.GoString(&result.devname[0]),
		UUID:       C.GoString(&result.uuid[0]),
		Type:       C.GoString(&result.fs_type[0]),
		Label:      C.GoString(&result.label[0]),
		Version:    C.GoString(&result.version[0]),
		PartUUID:   C.GoString(&result.part_uuid[0]),
		PartLabel:  C.GoString(&result.part_label[0]),
		MountFlags: int(result.mount_flags),
		MountPoint: C.GoString(&result.mount_point[0]),
	}, nil
}
