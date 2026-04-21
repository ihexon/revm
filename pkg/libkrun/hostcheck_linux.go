//go:build linux && (arm64 || amd64)

package libkrun

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

func CheckHostSupport() error {
	fd, err := unix.Open("/dev/kvm", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return wrapKVMError("open /dev/kvm", err)
	}
	defer unix.Close(fd)

	return nil
}

func wrapKVMError(op string, err error) error {
	var parts []string
	parts = append(parts, fmt.Sprintf("linux host validation failed: cannot %s: %v", op, err))
	parts = append(parts, "revm on Linux requires a working /dev/kvm")

	switch {
	case errors.Is(err, unix.ENOENT):
		parts = append(parts, "KVM is not available on this host")
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		parts = append(parts, "ensure your user can access /dev/kvm (typically by joining the kvm group)")
	}

	return errors.New(strings.Join(parts, "; "))
}
