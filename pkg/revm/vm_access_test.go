//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	libarchive_go "linuxvm/pkg/libarchive"
	"os"
	"path/filepath"
	"testing"
)

func TestExportRootfsFromSessionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootfsDir := filepath.Join(home, ".cache", "revm", "myengine", "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfsDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfsDir, "bin", "hello"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(rootfsDir, "usr", "share", "ca-certificates", "mozilla", "NetLock_Arany_=Class_Gold=_Főtanúsítvány.crt")
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, []byte("cert\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hello", filepath.Join(rootfsDir, "bin", "hello-link")); err != nil {
		t.Fatal(err)
	}
	sparsePath := filepath.Join(rootfsDir, "var", "lib", "sparse-file")
	if err := os.MkdirAll(filepath.Dir(sparsePath), 0755); err != nil {
		t.Fatal(err)
	}
	sparseFile, err := os.Create(sparsePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sparseFile.Seek(1<<20, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := sparseFile.Write([]byte("end")); err != nil {
		t.Fatal(err)
	}
	if err := sparseFile.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithRootfsExport(outputPath)

	if err := ExportRootfs(context.Background(), *cfg); err != nil {
		t.Fatalf("ExportRootfs() error = %v", err)
	}

	assertZstdArchive(t, outputPath)

	extractDir := t.TempDir()
	if err := libarchive_go.NewArchiver().
		WithArchiveFilePath(outputPath).
		SetChdir(extractDir).
		ModeX(context.Background()); err != nil {
		t.Fatalf("extract exported rootfs: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(extractDir, "bin", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("bin/hello content = %q, want hello", string(data))
	}
	certData, err := os.ReadFile(filepath.Join(extractDir, "usr", "share", "ca-certificates", "mozilla", "NetLock_Arany_=Class_Gold=_Főtanúsítvány.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(certData) != "cert\n" {
		t.Fatalf("non-ASCII cert content = %q, want cert", string(certData))
	}
	link, err := os.Readlink(filepath.Join(extractDir, "bin", "hello-link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "hello" {
		t.Fatalf("bin/hello-link = %q, want hello", link)
	}
	if _, err := os.Stat(filepath.Join(extractDir, "rootfs", "bin", "hello")); err == nil {
		t.Fatal("archive unexpectedly includes rootfs/ prefix")
	}

	assertSparseFileContent(t, filepath.Join(extractDir, "var", "lib", "sparse-file"))
}

func TestExportRootfsRejectsOutputInsideRootfs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootfsDir := filepath.Join(home, ".cache", "revm", "myengine", "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithRootfsExport(filepath.Join(rootfsDir, "rootfs.tar.zst"))

	if err := ExportRootfs(context.Background(), *cfg); err == nil {
		t.Fatal("ExportRootfs() accepted output path inside rootfs")
	}
}

func TestImportRootfsToSessionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "bin", "imported"), []byte("imported\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(srcDir, "bin", "imported"), filepath.Join(srcDir, "bin", "imported-hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("imported", filepath.Join(srcDir, "bin", "imported-link")); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "rootfs.tar.zst")
	if err := libarchive_go.NewArchiver().
		WithArchiveFilePath(archivePath).
		SetChdir(srcDir).
		ModeC(context.Background()); err != nil {
		t.Fatalf("create import archive: %v", err)
	}

	rootfsDir := filepath.Join(home, ".cache", "revm", "myengine", "rootfs")
	if err := os.MkdirAll(filepath.Join(rootfsDir, "old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfsDir, "old", "file"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithRootfsImport(archivePath)
	if err := ImportRootfs(context.Background(), *cfg); err != nil {
		t.Fatalf("ImportRootfs() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rootfsDir, "bin", "imported"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "imported\n" {
		t.Fatalf("imported content = %q, want imported", string(data))
	}
	link, err := os.Readlink(filepath.Join(rootfsDir, "bin", "imported-link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "imported" {
		t.Fatalf("imported-link = %q, want imported", link)
	}
	importedInfo, err := os.Stat(filepath.Join(rootfsDir, "bin", "imported"))
	if err != nil {
		t.Fatal(err)
	}
	hardlinkInfo, err := os.Stat(filepath.Join(rootfsDir, "bin", "imported-hardlink"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(importedInfo, hardlinkInfo) {
		t.Fatal("imported hardlink was not preserved")
	}
	if _, err := os.Stat(filepath.Join(rootfsDir, "old", "file")); err == nil {
		t.Fatal("old rootfs content still exists after import")
	}
}

func TestImportRootfsRejectsMissingArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithRootfsImport(filepath.Join(t.TempDir(), "missing.tar.zst"))

	if err := ImportRootfs(context.Background(), *cfg); err == nil {
		t.Fatal("ImportRootfs() accepted missing archive")
	}
}

func TestImportRootfsRejectsArchiveInsideTargetRootfs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootfsDir := filepath.Join(home, ".cache", "revm", "myengine", "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(rootfsDir, "rootfs.tar.zst")
	if err := os.WriteFile(archivePath, []byte("not an archive"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig().
		WithSessionID("myengine").
		WithRootfsImport(archivePath)

	if err := ImportRootfs(context.Background(), *cfg); err == nil {
		t.Fatal("ImportRootfs() accepted archive inside target rootfs")
	}
}

func assertZstdArchive(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 {
		t.Fatalf("archive is too small: %d bytes", len(data))
	}
	if data[0] != 0x28 || data[1] != 0xb5 || data[2] != 0x2f || data[3] != 0xfd {
		t.Fatalf("archive magic = % x, want zstd magic 28 b5 2f fd", data[:4])
	}
}

func assertSparseFileContent(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != (1<<20)+3 {
		t.Fatalf("sparse-file size = %d, want %d", info.Size(), (1<<20)+3)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	head := make([]byte, 3)
	if _, err := file.ReadAt(head, 0); err != nil {
		t.Fatal(err)
	}
	if string(head) != "\x00\x00\x00" {
		t.Fatalf("sparse-file head = %q, want zero padding", string(head))
	}

	tail := make([]byte, 3)
	if _, err := file.ReadAt(tail, 1<<20); err != nil {
		t.Fatal(err)
	}
	if string(tail) != "end" {
		t.Fatalf("sparse-file tail = %q, want end", string(tail))
	}
}
