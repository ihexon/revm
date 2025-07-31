package system

/*
#cgo CFLAGS: -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework CoreFoundation -framework SystemConfiguration

#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>


typedef enum {
    SUCCESS = 0,
    ERROR_FILE_ACCESS,
    ERROR_STREAM_CREATE,
    ERROR_PROPERTY_LIST,
    ERROR_INVALID_TYPE,
    ERROR_STRING_CONVERSION
} SystemInfoError;

typedef struct {
    char productName[256];
    char productVersion[256];
    char buildVersion[256];
    SystemInfoError error;
} SystemInfo;


static void cleanupResources(CFPropertyListRef propertyList, CFReadStreamRef stream,
                             CFURLRef fileURL) {
    if (propertyList) CFRelease(propertyList);
    if (stream) {
        CFReadStreamClose(stream);
        CFRelease(stream);
    }
    if (fileURL) CFRelease(fileURL);
}

static SystemInfo *allocateSystemInfo(void) {
    SystemInfo *info = calloc(1, sizeof(SystemInfo));
    if (!info) {
        info = NULL;
        perror("Memory allocation failed");
        return NULL;
    }
    info->error = SUCCESS;
    return info;
}

static bool getStringFromDict(CFDictionaryRef dict, const char *key, char *buffer, size_t bufferSize) {
    if (!dict || !key || !buffer || bufferSize == 0) {
        return false;
    }

    CFStringRef cfKey = CFStringCreateWithCString(kCFAllocatorDefault, key, kCFStringEncodingUTF8);
    if (!cfKey) {
        return false;
    }

    bool success = false;
    CFStringRef value = CFDictionaryGetValue(dict, cfKey);
    if (value) {
        success = CFStringGetCString(value, buffer, bufferSize, kCFStringEncodingUTF8);
    }

    CFRelease(cfKey);
    return success;
}

SystemInfo *getSystemInfo() {
    SystemInfo *info = allocateSystemInfo();
    if (!info) {
        return NULL;
    }

    CFURLRef fileURL = NULL;
    CFReadStreamRef stream = NULL;
    CFPropertyListRef propertyList = NULL;

    fileURL = CFURLCreateWithFileSystemPath(kCFAllocatorDefault,
                                            CFSTR("/System/Library/CoreServices/SystemVersion.plist"),
                                            kCFURLPOSIXPathStyle,
                                            false);
    if (!fileURL) {
        cleanupResources(propertyList, stream, fileURL);
        info->error = ERROR_FILE_ACCESS;
        return info;
    }

    stream = CFReadStreamCreateWithFile(kCFAllocatorDefault, fileURL);
    if (!stream || !CFReadStreamOpen(stream)) {
        cleanupResources(propertyList, stream, fileURL);
        info->error = ERROR_STREAM_CREATE;
        return info;
    }

    CFErrorRef error = NULL;
    propertyList = CFPropertyListCreateWithStream(kCFAllocatorDefault,
                                                  stream,
                                                  0,
                                                  kCFPropertyListImmutable,
                                                  NULL,
                                                  &error);
    if (!propertyList) {
        cleanupResources(propertyList, stream, fileURL);
        info->error = ERROR_PROPERTY_LIST;
        if (error) CFRelease(error);
        return info;
    }

    if (CFGetTypeID(propertyList) != CFDictionaryGetTypeID()) {
        cleanupResources(propertyList, stream, fileURL);
        info->error = ERROR_INVALID_TYPE;
        return info;
    }

    CFDictionaryRef dict = (CFDictionaryRef) propertyList;
    bool success = getStringFromDict(dict, "ProductName", info->productName, sizeof(info->productName)) &&
                   getStringFromDict(dict, "ProductVersion", info->productVersion, sizeof(info->productVersion)) &&
                   getStringFromDict(dict, "ProductBuildVersion", info->buildVersion, sizeof(info->buildVersion));

    if (!success) {
        cleanupResources(propertyList, stream, fileURL);
        info->error = ERROR_STRING_CONVERSION;
        return info;
    };

    cleanupResources(propertyList, stream, fileURL);
    return info;
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
	info := C.getSystemInfo()
	if info == nil {
		return nil, fmt.Errorf("failed to get system info")
	}
	defer C.free(unsafe.Pointer(info))

	if info.error != C.SUCCESS {
		return nil, fmt.Errorf("failed to get system info, return: %v", info.error)
	}

	// Safer string conversion with nil checks
	productVersion := (*C.char)(unsafe.Pointer(&info.productVersion[0]))
	buildVersion := (*C.char)(unsafe.Pointer(&info.buildVersion[0]))
	productName := (*C.char)(unsafe.Pointer(&info.productName[0]))

	if productVersion == nil {
		return nil, fmt.Errorf("invalid productVersion fields")
	}
	if buildVersion == nil {
		return nil, fmt.Errorf("invalid buildVersion fields")
	}
	if productName == nil {
		return nil, fmt.Errorf("invalid productName fields")
	}

	return &Info{
		ProductVersion:      C.GoString(&info.productVersion[0]),
		ProductBuildVersion: C.GoString(&info.buildVersion[0]),
		ProductName:         C.GoString(&info.productName[0]),
	}, nil
}
