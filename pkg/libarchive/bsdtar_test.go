package libarchive_go

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
