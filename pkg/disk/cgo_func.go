package disk

/*
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <ext2fs/ext2fs.h>
#include <blkid/blkid.h>
#include <uuid/uuid.h>

// Probe result structure with flattened fields
typedef struct {
    int error_code;
    char error_msg[256];
    // Filesystem info
    char uuid[64];
    char fs_type[64];
} blkid_probe_result;

// Probe the filesystem type and UUID.
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
        }
    }
    blkid_tag_iterate_end(iter);

    blkid_put_cache(cache);
    result.error_code = 0;
    return result;
}

// Result structure for ext2fs operations
typedef struct {
    int error_code;
    char error_msg[256];
} ext2fs_result;

// Set filesystem UUID (matches tune2fs.c behavior)
ext2fs_result ext2fs_set_uuid(const char* device, const char* new_uuid_str) {
    ext2fs_result result = {0};
    ext2_filsys fs = NULL;
    errcode_t retval;
    uuid_t new_uuid;
    dgrp_t i;
    int set_csum = 0;

    // Open the filesystem for writing
    retval = ext2fs_open(device, EXT2_FLAG_RW | EXT2_FLAG_64BITS, 0, 0, unix_io_manager, &fs);
    if (retval) {
        result.error_code = 1;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Failed to open filesystem: error %ld", (long)retval);
        return result;
    }

    // tune2fs.c line 3329: Clear MASTER_SB_ONLY immediately after open
    // This ensures backup superblocks are also updated
    fs->flags &= ~EXT2_FLAG_MASTER_SB_ONLY;

    // tune2fs.c line 3379: Set SUPER_ONLY (only write superblock by default)
    fs->flags |= EXT2_FLAG_SUPER_ONLY;

    // tune2fs.c line 3564-3569: Check stable_inodes
    if (ext2fs_has_feature_stable_inodes(fs->super)) {
        result.error_code = 2;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Cannot change UUID: filesystem has stable_inodes feature");
        ext2fs_close_free(&fs);
        return result;
    }

    // tune2fs.c line 3571-3580: Check if full metadata rewrite needed
    // We don't support this, require csum_seed
    if (!ext2fs_has_feature_csum_seed(fs->super) &&
        (ext2fs_has_feature_metadata_csum(fs->super) ||
         ext2fs_has_feature_ea_inode(fs->super))) {
        result.error_code = 5;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Filesystem needs metadata_csum_seed feature. "
                 "Use full tune2fs or run: tune2fs -O metadata_csum_seed");
        ext2fs_close_free(&fs);
        return result;
    }

    // tune2fs.c line 3609-3613: Verify group desc checksums before modifying
    if (ext2fs_has_group_desc_csum(fs)) {
        for (i = 0; i < fs->group_desc_count; i++) {
            if (!ext2fs_group_desc_csum_verify(fs, i))
                break;
        }
        if (i >= fs->group_desc_count)
            set_csum = 1;
    }

    if (uuid_parse(new_uuid_str, new_uuid) != 0) {
        result.error_code = 3;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Invalid UUID format: %s", new_uuid_str);
        ext2fs_close_free(&fs);
        return result;
    }

    // tune2fs.c line 3664: Copy new UUID to superblock
    memcpy(fs->super->s_uuid, new_uuid, sizeof(new_uuid));

    // tune2fs.c line 3665: Always reinitialize csum_seed
    ext2fs_init_csum_seed(fs);

    // tune2fs.c line 3666-3669: Update group desc checksums if valid
    if (set_csum) {
        for (i = 0; i < fs->group_desc_count; i++) {
            ext2fs_group_desc_csum_set(fs, i);
        }
        fs->flags &= ~EXT2_FLAG_SUPER_ONLY;
    }

    // tune2fs.c line 3671: Mark superblock dirty
    ext2fs_mark_super_dirty(fs);

    // Close and write changes
    retval = ext2fs_close_free(&fs);
    if (retval) {
        result.error_code = 4;
        snprintf(result.error_msg, sizeof(result.error_msg),
                 "Failed to close filesystem: error %ld", (long)retval);
        return result;
    }

    result.error_code = 0;
    return result;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

type probeResult struct {
	UUID string
	Type string
}

func probe(device string) (*probeResult, error) {
	cDevice := C.CString(device)
	defer C.free(unsafe.Pointer(cDevice))

	result := C.blkid_probe_device(cDevice)
	if result.error_code != 0 {
		return nil, errors.New(C.GoString(&result.error_msg[0]))
	}

	return &probeResult{UUID: C.GoString(&result.uuid[0]), Type: C.GoString(&result.fs_type[0])}, nil
}

// SetUUID sets the UUID of an ext2/3/4 filesystem
func SetUUID(device, newUUID string) error {
	cDevice := C.CString(device)
	cNewUUID := C.CString(newUUID)
	defer C.free(unsafe.Pointer(cDevice))
	defer C.free(unsafe.Pointer(cNewUUID))

	result := C.ext2fs_set_uuid(cDevice, cNewUUID)
	if result.error_code != 0 {
		return errors.New(C.GoString(&result.error_msg[0]))
	}
	return nil
}
