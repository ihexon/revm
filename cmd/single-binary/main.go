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
	"golang.org/x/sys/unix"
)

//go:embed payload.tar
var payloadTar []byte

var Name = "revm-single"

type Usage struct {
	Path        string
	TotalBytes  uint64
	UsedBytes   uint64
	FreeBytes   uint64
	AvailBytes  uint64
	UsedPercent float64
}

// GetDiskUsage returns filesystem usage of the mount containing path.
// Works on Linux and macOS.
func GetDiskUsage(path string) (*Usage, error) {
	var stat unix.Statfs_t

	if err := unix.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs failed for %s: %w", path, err)
	}

	blockSize := uint64(stat.Bsize)

	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	avail := stat.Bavail * blockSize
	used := total - free

	var usedPercent float64
	if total > 0 {
		usedPercent = (float64(used) / float64(total)) * 100
	}

	return &Usage{
		Path:        path,
		TotalBytes:  total,
		UsedBytes:   used,
		FreeBytes:   free,
		AvailBytes:  avail,
		UsedPercent: usedPercent,
	}, nil
}

func main() {
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func BytesToMB(b uint64) float64 {
	return float64(b) / 1_000_000
}

func BytesToGB(b uint64) float64 {
	return float64(b) / 1_000_000_000
}

func checkDiskSpace() error {
	usage, err := GetDiskUsage("/tmp")
	if err != nil {
		return fmt.Errorf("get disk usage failed: %w", err)
	}

	if BytesToMB(usage.AvailBytes) < 512 {
		return fmt.Errorf("/tmp insufficient disk space: %d bytes available", usage.AvailBytes)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir failed: %w", err)
	}

	usage, err = GetDiskUsage(homeDir)
	if err != nil {
		return fmt.Errorf("get disk usage failed: %w", err)
	}

	if BytesToGB(usage.AvailBytes) < 1 {
		return fmt.Errorf("%q insufficient disk space: %d bytes available", homeDir, usage.AvailBytes)
	}

	return nil
}

func run() error {
	if err := checkDiskSpace(); err != nil {
		return err
	}

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
