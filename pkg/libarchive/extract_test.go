package libarchive_go

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func archiveBytes(t *testing.T, entries ...tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, h := range entries {
		if err := w.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRootFSMetadataAndLinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := archiveBytes(t,
		tar.Header{Name: "日本語", Mode: 0444, PAXRecords: map[string]string{"SCHILY.xattr.user.revm-test": "1000"}},
		tar.Header{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "日本語"},
		tar.Header{Name: "symlink", Typeflag: tar.TypeSymlink, Mode: 0777, Linkname: "/日本語"},
	)
	if err := ExtractRootFS(context.Background(), data, dir); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(filepath.Join(dir, "日本語"))
	if err != nil {
		t.Fatal(err)
	}
	linked, err := os.Stat(filepath.Join(dir, "hardlink"))
	if err != nil {
		t.Fatal(err)
	}
	if original.Mode().Perm() != 0444 || !os.SameFile(original, linked) {
		t.Fatal("lost mode or hardlink")
	}
	buf := make([]byte, 100)
	n, err := unix.Getxattr(filepath.Join(dir, "日本語"), "user.revm-test", buf)
	if err != nil || string(buf[:n]) != "1000" {
		t.Fatalf("lost xattr: %q, %v", buf, err)
	}
	if link, err := os.Readlink(filepath.Join(dir, "symlink")); err != nil || link != "/日本語" {
		t.Fatalf("symlink: %q %v", link, err)
	}
}

func TestRejectArchiveEscapes(t *testing.T) {
	t.Parallel()
	for _, h := range []tar.Header{
		{Name: "../escape"}, {Name: "/escape"},
		{Name: "hardlink", Typeflag: tar.TypeLink, Linkname: "../escape"},
	} {
		if err := ExtractRootFS(context.Background(), archiveBytes(t, h), t.TempDir()); err == nil {
			t.Fatalf("accepted %+v", h)
		}
	}
	dir, outside := t.TempDir(), t.TempDir()
	data := archiveBytes(t, tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Mode: 0777, Linkname: outside}, tar.Header{Name: "link/escape"})
	if err := ExtractRootFS(context.Background(), data, dir); err == nil {
		t.Fatal("followed symlink outside destination")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatal("wrote outside destination")
	}
}

func TestDiskExtraction(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "disk.ext4")
	if err := ExtractDisk(context.Background(), archiveBytes(t, tar.Header{Name: "unrelated"}), target); err == nil {
		t.Fatal("accepted archive without disk")
	}
	if err := ExtractDisk(context.Background(), archiveBytes(t, tar.Header{Name: "ext4.raw", Mode: 0600}), target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}
