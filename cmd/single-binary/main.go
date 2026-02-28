package main

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

//go:embed payload.tar
var payloadTar []byte

var buildID = "dev" // set via -ldflags at build time

func main() {
	cacheDir := filepath.Join(os.TempDir(), ".revm-"+buildID)
	if err := ensureExtracted(cacheDir); err != nil {
		fmt.Fprintln(os.Stderr, "revm-single:", err)
		os.Exit(1)
	}
	bin := filepath.Join(cacheDir, "bin", "revm")
	args := append([]string{bin}, os.Args[1:]...)
	env := os.Environ()

	switch runtime.GOOS {
	case "linux":
		ldLinux := filepath.Join(cacheDir, "helper", "ld-linux-aarch64.so.1")
		libDir := filepath.Join(cacheDir, "lib")
		_ = syscall.Exec(ldLinux, append([]string{"ld-linux-aarch64.so.1", "--library-path", libDir}, args...), env)
	default:
		_ = syscall.Exec(bin, args, env)
	}
	fmt.Fprintln(os.Stderr, "revm-single: exec failed")
	os.Exit(1)
}

func ensureExtracted(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "bin", "revm")); err == nil {
		return nil
	}
	tmp := fmt.Sprintf("%s.tmp.%d", dir, os.Getpid())
	os.RemoveAll(tmp)
	start := time.Now()
	if err := extractTo(tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	fmt.Fprintf(os.Stderr, "revm-single: extracted payload in %s\n", time.Since(start))
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
	tr := tar.NewReader(bytes.NewReader(payloadTar))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		name := filepath.Clean(hdr.Name)
		dst := filepath.Join(dir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, dst); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}
