//go:build darwin

package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed payload
var payload embed.FS

func main() {
	cacheDir := filepath.Join(os.TempDir(), ".revm-"+buildID())
	if err := ensureExtracted(cacheDir); err != nil {
		fmt.Fprintln(os.Stderr, "revm-single:", err)
		os.Exit(1)
	}
	bin := filepath.Join(cacheDir, "bin", "revm")
	_ = syscall.Exec(bin, append([]string{bin}, os.Args[1:]...), os.Environ())
	fmt.Fprintln(os.Stderr, "revm-single: exec failed")
	os.Exit(1)
}

// buildID reads the content-hash written by build.sh at package time.
func buildID() string {
	b, err := payload.ReadFile("payload/build_id")
	if err != nil || len(b) == 0 {
		return "dev"
	}
	return strings.TrimSpace(string(b))
}

// ensureExtracted is idempotent: fast-path if bin/revm already exists,
// otherwise extracts atomically via a per-process temp dir + rename.
func ensureExtracted(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "bin", "revm")); err == nil {
		return nil // already extracted
	}
	tmp := fmt.Sprintf("%s.tmp.%d", dir, os.Getpid())
	os.RemoveAll(tmp)
	if err := extractTo(tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	// Atomic rename; tolerate a concurrent process that won the race.
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		if _, statErr := os.Stat(filepath.Join(dir, "bin", "revm")); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func extractTo(dir string) error {
	return fs.WalkDir(payload, "payload", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "payload" {
			return err
		}
		dst := filepath.Join(dir, path[len("payload/"):])
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		src, err := payload.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, src)
		return err
	})
}
