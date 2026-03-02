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

	"github.com/sirupsen/logrus"
)

//go:embed payload.tar
var payloadTar []byte

var Name = "revm-single"

func main() {
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	dir := os.Getenv("PAYLOAD_DIR")

	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", ".revm-*")
		if err != nil {
			return fmt.Errorf("mkdirtemp: %w", err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "bin", "revm")); err != nil {
		start := time.Now()
		if err := extractTo(dir); err != nil {
			return err
		}
		logrus.Infof("[%s] extracted payload in %s", Name, time.Since(start))
	}

	bin := filepath.Join(dir, "bin", "revm")
	args := append([]string{bin}, os.Args[1:]...)

	env := os.Environ()
	if os.Getenv("PAYLOAD_DIR") == "" {
		env = append(env, "PAYLOAD_DIR="+dir)
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH
	switch platform {
	case "linux/arm64":
		ldLinux := filepath.Join(dir, "helper", "ld-linux-aarch64.so.1")
		libDir := filepath.Join(dir, "lib")
		_ = syscall.Exec(ldLinux, append([]string{"ld-linux-aarch64.so.1", "--library-path", libDir}, args...), env)
	case "darwin/arm64":
		_ = syscall.Exec(bin, args, env)
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
	return fmt.Errorf("exec failed")
}

// TODO: using libarchive to speed up extraction, payload can be compressed mybe
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

		dst := filepath.Join(dir, filepath.Clean(hdr.Name))

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
