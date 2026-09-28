package libarchive_go

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestModeCArchivesUTF8Pathnames(t *testing.T) {
	srcDir := t.TempDir()
	name := "NetLock_Arany_=Class_Gold=_Főtanúsítvány.crt"
	if err := os.WriteFile(filepath.Join(srcDir, name), []byte("cert\n"), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := NewArchiver().
		WithArchiveFilePath(archivePath).
		SetChdir(srcDir).
		ModeC(context.Background()); err != nil {
		t.Fatalf("ModeC() error = %v", err)
	}

	extractDir := t.TempDir()
	if err := NewArchiver().
		WithArchiveFilePath(archivePath).
		SetChdir(extractDir).
		ModeX(context.Background()); err != nil {
		t.Fatalf("ModeX() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(extractDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "cert\n" {
		t.Fatalf("content = %q, want cert", string(data))
	}
}

func TestModeCRoundTripsVirtioFSOverrideStat(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "file")
	if err := os.WriteFile(srcPath, []byte("data\n"), 0644); err != nil {
		t.Fatal(err)
	}
	const key = "user.containers.override_stat"
	if err := unix.Setxattr(srcPath, key, []byte("0:0:0644"), 0); err != nil {
		if err == unix.ENOTSUP || err == unix.EOPNOTSUPP {
			t.Skipf("filesystem does not support xattrs: %v", err)
		}
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := NewArchiver().WithArchiveFilePath(archivePath).SetChdir(srcDir).ModeC(context.Background()); err != nil {
		t.Fatalf("ModeC() error = %v", err)
	}

	dstDir := t.TempDir()
	if err := NewArchiver().WithArchiveFilePath(archivePath).SetChdir(dstDir).IncludeFileAttribute().ModeX(context.Background()); err != nil {
		t.Fatalf("ModeX() error = %v", err)
	}
	size, err := unix.Getxattr(filepath.Join(dstDir, "file"), key, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := make([]byte, size)
	if _, err := unix.Getxattr(filepath.Join(dstDir, "file"), key, value); err != nil {
		t.Fatal(err)
	}
	if string(value) != "0:0:0644" {
		t.Fatalf("override stat = %q, want %q", value, "0:0:0644")
	}
}

func TestModeCPreservesHardlinks(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "original"), []byte("shared\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(srcDir, "original"), filepath.Join(srcDir, "linked")); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := NewArchiver().
		WithArchiveFilePath(archivePath).
		SetChdir(srcDir).
		ModeC(context.Background()); err != nil {
		t.Fatalf("ModeC() error = %v", err)
	}

	extractDir := t.TempDir()
	if err := NewArchiver().
		WithArchiveFilePath(archivePath).
		SetChdir(extractDir).
		ModeX(context.Background()); err != nil {
		t.Fatalf("ModeX() error = %v", err)
	}

	originalInfo, err := os.Stat(filepath.Join(extractDir, "original"))
	if err != nil {
		t.Fatal(err)
	}
	linkedInfo, err := os.Stat(filepath.Join(extractDir, "linked"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(originalInfo, linkedInfo) {
		t.Fatal("extracted files are not hardlinks to the same inode")
	}
}
