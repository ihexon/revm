package libarchive_go

/*
#include <archive.h>
#include <archive_entry.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"
)

type mode int
type format int

const (
	PAX format = iota
)

const (
	modeX mode = iota
)

// extractFlags for archive extraction
type extractFlags int

const (
	ExtractTime             extractFlags = C.ARCHIVE_EXTRACT_TIME
	ExtractPerm             extractFlags = C.ARCHIVE_EXTRACT_PERM
	ExtractOwner            extractFlags = C.ARCHIVE_EXTRACT_OWNER
	ExtractACL              extractFlags = C.ARCHIVE_EXTRACT_ACL
	ExtractXattr            extractFlags = C.ARCHIVE_EXTRACT_XATTR
	ExtractFFlags           extractFlags = C.ARCHIVE_EXTRACT_FFLAGS
	ExtractSecureSymlink    extractFlags = C.ARCHIVE_EXTRACT_SECURE_SYMLINKS
	ExtractSecureNoDot      extractFlags = C.ARCHIVE_EXTRACT_SECURE_NODOTDOT
	ExtractSecureNoAbsolute extractFlags = C.ARCHIVE_EXTRACT_SECURE_NOABSOLUTEPATHS
	ExtractUnlink           extractFlags = C.ARCHIVE_EXTRACT_UNLINK
	ExtractSparse           extractFlags = C.ARCHIVE_EXTRACT_SPARSE
)

// defaultExtractFlags: time + security checks (same as bsdtar for non-root)
const defaultExtractFlags = ExtractTime | ExtractSecureSymlink | ExtractSecureNoDot | ExtractSecureNoAbsolute | ExtractUnlink

// defaultBytesPerBlock is the read buffer size (256KB for better throughput)
const defaultBytesPerBlock = 256 * 1024

// Archiver provides tar archive operations
type Archiver struct {
	mode                 mode      // x, t
	filename             string    // if filename is '-' or empty, read from stdin
	reader               io.Reader // external data source (takes precedence over filename)
	pendingChdir         string
	safeWrite            bool
	format               format
	verbose              int
	patterns             []string // inclusion patterns (stored for lazy initialization)
	bytesPerBlock        int
	matching             *C.struct_archive // libarchive matching object
	fastRead             bool
	sparse               bool
	includeFileAttribute bool
	transform            map[string]string
}

func NewArchiver() *Archiver {
	return &Archiver{
		safeWrite:            true,
		format:               PAX,
		bytesPerBlock:        defaultBytesPerBlock,
		fastRead:             false,
		sparse:               false,
		includeFileAttribute: false,
	}
}

// WithArchiveFilePath sets the archive filename. Use "-" or empty for stdin.
func (t *Archiver) WithArchiveFilePath(filename string) *Archiver {
	t.filename = filename
	return t
}

// SetReader sets an io.Reader as the archive data source.
// When set, this takes precedence over filename.
func (t *Archiver) SetReader(r io.Reader) *Archiver {
	t.reader = r
	return t
}

// SetVerbose sets verbosity level
func (t *Archiver) SetVerbose(level int) *Archiver {
	t.verbose = level
	return t
}

func (t *Archiver) SetSparse(sparse bool) *Archiver {
	t.sparse = sparse
	return t
}

// SetBytesPerBlock sets the read buffer size for archive operations
func (t *Archiver) SetBytesPerBlock(size int) *Archiver {
	t.bytesPerBlock = size
	return t
}

// WithPattern adds an inclusion pattern for extraction using libarchive's pattern matching
func (t *Archiver) WithPattern(pattern string) *Archiver {
	t.patterns = append(t.patterns, pattern)
	return t
}

// initMatching initializes the libarchive matching object with stored patterns
func (t *Archiver) initMatching() error {
	t.matching = C.archive_match_new()
	if t.matching == nil {
		return errors.New("cannot allocate match object")
	}

	for _, pattern := range t.patterns {
		cPattern := C.CString(pattern)
		r := C.archive_match_include_pattern(t.matching, cPattern)
		C.free(unsafe.Pointer(cPattern))
		if r != C.ARCHIVE_OK {
			return fmt.Errorf("failed to add pattern '%s': %s",
				pattern, C.GoString(C.archive_error_string(t.matching)))
		}
	}
	return nil
}

// freeMatching releases the libarchive matching object
func (t *Archiver) freeMatching() {
	if t.matching != nil {
		C.archive_match_free(t.matching)
		t.matching = nil
	}
}

func (t *Archiver) SetFastRead(fastRead bool) *Archiver {
	t.fastRead = fastRead
	return t
}

func (t *Archiver) IncludeFileAttribute() *Archiver {
	t.includeFileAttribute = true
	return t
}

// WithTransform adds a pathname rename rule applied during extraction.
// Files with pathname matching oldName will be extracted as newName.
func (t *Archiver) WithTransform(oldName, newName string) *Archiver {
	if t.transform == nil {
		t.transform = make(map[string]string)
	}
	t.transform[oldName] = newName
	return t
}

func (t *Archiver) SetChdir(dir string) *Archiver {
	t.pendingChdir = dir
	return t
}

// doChdir executes any pending chdir request
func (t *Archiver) doChdir() error {
	if t.pendingChdir == "" {
		return nil
	}

	if err := os.Chdir(t.pendingChdir); err != nil {
		return fmt.Errorf("could not chdir to '%s': %w", t.pendingChdir, err)
	}
	t.pendingChdir = ""
	return nil
}

// ModeX extracts files from an archive (equivalent to tar -x)
func (t *Archiver) ModeX(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get current working directory: %w", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to restore original working directory: %v\n", err)
		}
	}()

	extractFlags := defaultExtractFlags

	// preserve permissions, owner, ACL, xattr, fflags (same as bsdtar -p)
	if t.includeFileAttribute {
		extractFlags |= ExtractPerm | ExtractOwner | ExtractACL | ExtractXattr | ExtractFFlags
	}

	if t.sparse {
		extractFlags |= ExtractSparse
	}

	// Initialize pattern matching
	if err := t.initMatching(); err != nil {
		return err
	}
	defer t.freeMatching()

	// Create disk writer
	writer := C.archive_write_disk_new()
	if writer == nil {
		return errors.New("cannot allocate disk writer object")
	}
	defer C.archive_write_free(writer)

	C.archive_write_disk_set_options(writer, C.int(extractFlags))

	return t.readArchive(ctx, writer)
}

func (t *Archiver) readArchive(ctx context.Context, writer *C.struct_archive) error {
	// Create archive reader
	a := C.archive_read_new()
	if a == nil {
		return errors.New("cannot allocate archive reader")
	}
	defer C.archive_read_free(a)

	// Support all formats and filters
	C.archive_read_support_filter_all(a)
	C.archive_read_support_format_all(a)

	// Both file and reader paths use a pipe so that ctx cancellation
	// can close the write end and interrupt blocking C read calls.
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	// sourceErrCh receives the error from the source goroutine (nil on success).
	// Buffered so the goroutine never blocks on send.
	sourceErrCh := make(chan error, 1)

	go func() {
		var copyErr error
		defer func() {
			_ = pw.Close()
			sourceErrCh <- copyErr
		}()

		// Monitor ctx in a separate goroutine; closing pw interrupts
		// any blocking read() in libarchive.
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = pw.Close() // safe: duplicate close is no-op after first
			case <-done:
			}
		}()

		var src io.Reader
		switch {
		case t.reader != nil:
			src = t.reader
		case t.filename == "" || t.filename == "-":
			src = os.Stdin
		default:
			f, err := os.Open(t.filename)
			if err != nil {
				copyErr = err
				return
			}
			defer func() {
				if err := f.Close(); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "failed to close file: %v\n", err)
				}
			}()
			src = f
		}
		_, copyErr = io.Copy(pw, src) // block until copy is complete or ctx is canceled
	}()

	r := C.archive_read_open_fd(a, C.int(pr.Fd()), C.size_t(t.bytesPerBlock))
	if r != C.ARCHIVE_OK {
		_ = pr.Close()
		if srcErr := <-sourceErrCh; srcErr != nil {
			return fmt.Errorf("error opening archive: %w", srcErr)
		}
		return fmt.Errorf("error opening archive: %v", C.GoString(C.archive_error_string(a)))
	}
	defer C.archive_read_close(a)
	defer func() { _ = pr.Close() }()

	// Execute pending chdir before processing entries
	if err := t.doChdir(); err != nil {
		return err
	}

	// Process entries
	var entry *C.struct_archive_entry
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if t.fastRead && C.archive_match_path_unmatched_inclusions(t.matching) == 0 {
			break
		}

		r = C.archive_read_next_header(a, &entry) //nolint:gocritic // false positive: dupSubExpr misreports CGo call site
		if r == C.ARCHIVE_EOF {
			break
		}

		if r == C.ARCHIVE_FATAL {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%v", C.GoString(C.archive_error_string(a)))
		}

		if r < C.ARCHIVE_OK {
			_, _ = fmt.Fprintf(os.Stderr, "warning: %v\n", C.GoString(C.archive_error_string(a)))
		}

		if r == C.ARCHIVE_RETRY {
			continue
		}

		pathname := C.GoString(C.archive_entry_pathname(entry))
		if pathname == "" {
			_, _ = fmt.Fprintf(os.Stderr, "warning: archive entry has empty or unreadable filename, skipping\n")
			continue
		}

		// Check inclusion/exclusion patterns using libarchive
		if t.matching != nil && C.archive_match_excluded(t.matching, entry) != 0 {
			C.archive_read_data_skip(a)
			continue
		}

		// Apply pathname transform rules
		if newName, ok := t.transform[pathname]; ok {
			if newName == "" {
				C.archive_read_data_skip(a)
				continue
			}
			cNewName := C.CString(newName)
			C.archive_entry_copy_pathname(entry, cNewName)
			C.free(unsafe.Pointer(cNewName))
			if t.verbose > 0 {
				_, _ = fmt.Fprintf(os.Stderr, "x %v -> %v\n", pathname, newName)
			}
		} else if t.verbose > 0 {
			_, _ = fmt.Fprintf(os.Stderr, "x %v\n", pathname)
		}

		if hl := C.GoString(C.archive_entry_hardlink(entry)); hl != "" {
			if newHL, ok := t.transform[hl]; ok {
				cNewHL := C.CString(newHL)
				C.archive_entry_copy_hardlink(entry, cNewHL)
				C.free(unsafe.Pointer(cNewHL))
			}
		}

		// Extract entry
		r = C.archive_read_extract2(a, entry, writer)
		if r != C.ARCHIVE_OK {
			errStr := C.GoString(C.archive_error_string(a))
			if r == C.ARCHIVE_FATAL {
				if ctx.Err() != nil {
					return ctx.Err()
				}
			}
			return fmt.Errorf("extract %v: %v", pathname, errStr)
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

func ShowVersion() {
	cVersion := C.archive_version_details()
	_, _ = fmt.Fprintf(os.Stderr, "%v\n", C.GoString(cVersion))
}
