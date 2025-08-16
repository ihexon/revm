//go:build darwin && (arm64 || amd64)

package system

/*
#cgo CFLAGS: -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework CoreFoundation -framework SystemConfiguration

#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include <string.h>


typedef enum {
    SYSINFO_OK = 0,
    SYSINFO_ERR_FILE_ACCESS,
    SYSINFO_ERR_STREAM_CREATE,
    SYSINFO_ERR_PROPERTY_LIST,
    SYSINFO_ERR_INVALID_TYPE,
    SYSINFO_ERR_STRING_CONVERSION,
	SYSINFO_ERR_INVALID_ARGUMENT
} sysinfo_error_t;

typedef struct {
    char productName[256];
    char productVersion[256];
    char buildVersion[256];
} sysinfo_t;

typedef struct {
    CFURLRef fileURL;
    CFReadStreamRef stream;
    CFPropertyListRef propertyList;
} sysinfo_cf_handles_t;


static void sysinfo_cleanup_handles(sysinfo_cf_handles_t *h) {
    if (!h) return;
    if (h->propertyList) CFRelease(h->propertyList);
    if (h->stream) {
        CFReadStreamClose(h->stream);
        CFRelease(h->stream);
    }
    if (h->fileURL) CFRelease(h->fileURL);
}

static bool sysinfo_get_string(CFDictionaryRef dict, CFStringRef key, char *out_buf, size_t buf_size) {
    if (!dict || !key || !out_buf || buf_size == 0) return false;
    CFStringRef value = CFDictionaryGetValue(dict, key);
    return value && CFStringGetCString(value, out_buf, buf_size, kCFStringEncodingUTF8);
}


sysinfo_error_t sysinfo_load(sysinfo_t *out_info) {
    if (!out_info) return SYSINFO_ERR_INVALID_ARGUMENT;
    memset(out_info, 0, sizeof(*out_info));

    sysinfo_cf_handles_t h = {0}; // 初始化 CoreFoundation 句柄

    h.fileURL = CFURLCreateWithFileSystemPath(
        kCFAllocatorDefault,
        CFSTR("/System/Library/CoreServices/SystemVersion.plist"),
        kCFURLPOSIXPathStyle,
        false
    );
    if (!h.fileURL) {
        sysinfo_cleanup_handles(&h);
        return SYSINFO_ERR_FILE_ACCESS;
    }

    h.stream = CFReadStreamCreateWithFile(kCFAllocatorDefault, h.fileURL);
    if (!h.stream || !CFReadStreamOpen(h.stream)) {
        sysinfo_cleanup_handles(&h);
        return SYSINFO_ERR_STREAM_CREATE;
    }

    CFErrorRef cfErr = NULL;
    h.propertyList = CFPropertyListCreateWithStream(
        kCFAllocatorDefault,
        h.stream,
        0,
        kCFPropertyListImmutable,
        NULL,
        &cfErr
    );
    if (!h.propertyList) {
        if (cfErr) CFRelease(cfErr);
        sysinfo_cleanup_handles(&h);
        return SYSINFO_ERR_PROPERTY_LIST;
    }

    if (CFGetTypeID(h.propertyList) != CFDictionaryGetTypeID()) {
        sysinfo_cleanup_handles(&h);
        return SYSINFO_ERR_INVALID_TYPE;
    }

    CFDictionaryRef dict = (CFDictionaryRef)h.propertyList;
    bool ok =
        sysinfo_get_string(dict, CFSTR("ProductName"), out_info->productName, sizeof(out_info->productName)) &&
        sysinfo_get_string(dict, CFSTR("ProductVersion"), out_info->productVersion, sizeof(out_info->productVersion)) &&
        sysinfo_get_string(dict, CFSTR("ProductBuildVersion"), out_info->buildVersion, sizeof(out_info->buildVersion));

    sysinfo_cleanup_handles(&h);
    return ok ? SYSINFO_OK : SYSINFO_ERR_STRING_CONVERSION;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type Info struct {
	ProductName         string
	ProductVersion      string
	ProductBuildVersion string
}

func GetOSVersion() (*Info, error) {
	var info C.sysinfo_t
	ret := C.sysinfo_load((*C.sysinfo_t)(unsafe.Pointer(&info)))
	if ret != C.SYSINFO_OK {
		return nil, fmt.Errorf("failed to get os version, return %v", ret)
	}

	return &Info{
		ProductName:         C.GoString(&info.productName[0]),
		ProductVersion:      C.GoString(&info.productVersion[0]),
		ProductBuildVersion: C.GoString(&info.buildVersion[0]),
	}, nil
}
