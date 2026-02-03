package service

import (
	_ "embed"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
)

//go:embed busybox.static
var busyboxBytes []byte

//go:embed dropbearmulti
var dropbearmultiBytes []byte

// embeddedBinary represents an embedded binary that can be extracted to disk.
type embeddedBinary struct {
	name  string
	bytes []byte
}

var (
	busyboxBinary       = &embeddedBinary{name: "busybox", bytes: busyboxBytes}
	dropbearmultiBinary = &embeddedBinary{name: "dropbearmulti", bytes: dropbearmultiBytes}
)

// InitBinDir mounts tmpfs to /.bin and extracts all embedded binaries.
// This must be called early in the boot process before any other services.
func InitBinDir() error {
	binDir := define.GuestHiddenBinDir

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	if err := syscall.Mount("tmpfs", binDir, "tmpfs", 0, "mode=0755"); err != nil {
		return fmt.Errorf("mount tmpfs to %s: %w", binDir, err)
	}

	// Extract all binaries
	binaries := []*embeddedBinary{busyboxBinary, dropbearmultiBinary}
	for _, bin := range binaries {
		path := filepath.Join(binDir, bin.name)
		if err := os.WriteFile(path, bin.bytes, 0755); err != nil {
			return fmt.Errorf("extract %s: %w", bin.name, err)
		}
		logrus.Infof("extracted %q to %q", bin.name, path)
	}

	return nil
}

// BusyboxPath returns the path to the busybox binary.
func BusyboxPath() string {
	return filepath.Join(define.GuestHiddenBinDir, busyboxBinary.name)
}

// DropbearmultiPath returns the path to the dropbearmulti binary.
func DropbearmultiPath() string {
	return filepath.Join(define.GuestHiddenBinDir, dropbearmultiBinary.name)
}
