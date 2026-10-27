package libarchive_go

/*
#include <archive.h>
#include <archive_entry.h>
#include <locale.h>
#include <stdlib.h>
#ifdef __APPLE__
#include <xlocale.h>
#define REVM_UTF8_LOCALE "UTF-8"
#else
#define REVM_UTF8_LOCALE "C.UTF-8"
#endif

static locale_t extraction_locale(void) {
	return newlocale(LC_CTYPE_MASK, REVM_UTF8_LOCALE, (locale_t)0);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ExtractRootFS(ctx context.Context, data []byte, dir string) error {
	return extract(ctx, data, dir, false)
}

func ExtractDisk(ctx context.Context, data []byte, filename string) error {
	return extract(ctx, data, filename, true)
}

// Both inputs are embedded tar archives. Disk extraction selects ext4.raw;
// rootfs extraction also restores libkrun's ownership metadata in xattrs.
func extract(ctx context.Context, data []byte, destination string, disk bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("embedded archive is empty")
	}
	// Tar pathnames use UTF-8 regardless of the caller's locale. Keep the
	// locale thread-local so concurrent native code is not affected.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	locale := C.extraction_locale()
	if locale == nil {
		return errors.New("cannot create UTF-8 locale for archive extraction")
	}
	previous := C.uselocale(locale)
	defer func() {
		C.uselocale(previous)
		C.freelocale(locale)
	}()
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	dir := destination
	if disk {
		dir = filepath.Dir(destination)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Resolve existing parent symlinks before enabling the disk writer's
	// protection against following symlinks created by archive entries.
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}

	var pin runtime.Pinner
	pin.Pin(&data[0])
	defer pin.Unpin()
	reader := C.archive_read_new()
	if reader == nil {
		return errors.New("cannot allocate archive reader")
	}
	defer C.archive_read_free(reader)
	C.archive_read_support_filter_all(reader)
	C.archive_read_support_format_tar(reader)
	if C.archive_read_open_memory(reader, unsafe.Pointer(&data[0]), C.size_t(len(data))) != C.ARCHIVE_OK {
		return fmt.Errorf("open archive: %s", C.GoString(C.archive_error_string(reader)))
	}
	writer := C.archive_write_disk_new()
	if writer == nil {
		return errors.New("cannot allocate archive writer")
	}
	defer C.archive_write_free(writer)
	C.archive_write_disk_set_options(writer, C.ARCHIVE_EXTRACT_TIME|C.ARCHIVE_EXTRACT_PERM|C.ARCHIVE_EXTRACT_SPARSE|C.ARCHIVE_EXTRACT_SECURE_SYMLINKS|C.ARCHIVE_EXTRACT_SECURE_NODOTDOT)

	var entry *C.struct_archive_entry
	found := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status := C.archive_read_next_header(reader, &entry)
		if status == C.ARCHIVE_EOF {
			break
		}
		if status != C.ARCHIVE_OK {
			return fmt.Errorf("read archive: %s", C.GoString(C.archive_error_string(reader)))
		}
		name := C.GoString(C.archive_entry_pathname(entry))
		if !filepath.IsLocal(name) {
			return fmt.Errorf("invalid archive path %q", name)
		}
		if disk && name != "ext4.raw" {
			continue
		}
		target := filepath.Join(dir, name)
		if disk {
			target = filepath.Join(dir, filepath.Base(destination))
		}
		pathname := C.CString(target)
		C.archive_entry_copy_pathname(entry, pathname)
		C.free(unsafe.Pointer(pathname))
		if link := C.GoString(C.archive_entry_hardlink(entry)); link != "" {
			if disk || !filepath.IsLocal(link) {
				return fmt.Errorf("invalid archive hardlink %q", link)
			}
			hardlink := C.CString(filepath.Join(dir, link))
			C.archive_entry_copy_hardlink(entry, hardlink)
			C.free(unsafe.Pointer(hardlink))
		}
		if C.archive_read_extract2(reader, entry, writer) != C.ARCHIVE_OK {
			return fmt.Errorf("extract %s: %s", name, C.GoString(C.archive_error_string(reader)))
		}
		if !disk {
			if err := restoreEntryXattrs(target, entry); err != nil {
				return fmt.Errorf("restore xattrs for %s: %w", name, err)
			}
		}
		found = true
		if disk {
			break
		}
	}
	if !found {
		return errors.New("embedded archive contains no expected files")
	}
	if C.archive_write_close(writer) != C.ARCHIVE_OK {
		return fmt.Errorf("finish extraction: %s", C.GoString(C.archive_error_string(writer)))
	}
	return ctx.Err()
}

func restoreEntryXattrs(pathname string, entry *C.struct_archive_entry) error {
	if C.archive_entry_xattr_count(entry) == 0 {
		return nil
	}

	info, err := os.Lstat(pathname)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	originalMode := info.Mode().Perm()
	writableMode := originalMode | 0200
	if writableMode != originalMode {
		if err := os.Chmod(pathname, writableMode); err != nil {
			return err
		}
		defer func() { _ = os.Chmod(pathname, originalMode) }()
	}

	if C.archive_entry_xattr_reset(entry) < 0 {
		return errors.New("cannot enumerate extended attributes")
	}
	for {
		var name *C.char
		var value unsafe.Pointer
		var size C.size_t
		next := C.archive_entry_xattr_next(entry, &name, &value, &size)
		if next != C.ARCHIVE_OK {
			// archive_entry_xattr_next returns a negative status after the
			// final attribute (currently ARCHIVE_WARN in libarchive).
			break
		}
		if name == nil {
			return errors.New("archive entry has an empty extended-attribute name")
		}
		data := unsafe.Slice((*byte)(value), int(size))
		if err := unix.Lsetxattr(pathname, C.GoString(name), data, 0); err != nil {
			return fmt.Errorf("%s: %w", C.GoString(name), err)
		}
	}
	return nil
}
