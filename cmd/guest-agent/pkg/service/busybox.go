package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// Exec runs a busybox command with the given arguments.
func Exec(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, BusyboxPath(), args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Run()
}

// ExecOutput runs a busybox command and captures output to the provided writers.
func ExecOutput(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, BusyboxPath(), args...)
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Run()
}

// Mount runs busybox mount command.
func Mount(ctx context.Context, args ...string) error {
	return Exec(ctx, append([]string{"mount"}, args...)...)
}

// Umount runs busybox umount command.
func Umount(ctx context.Context, target string) error {
	return Exec(ctx, "umount", "-l", "-d", "-f", target)
}

// IsMounted checks if a path is a mount point.
func IsMounted(target string) bool {
	if err := Exec(context.Background(), "mountpoint", "-q", target); err != nil {
		return false
	}
	return true
}

// Hostname sets the system hostname.
func Hostname(ctx context.Context, name string) error {
	return Exec(ctx, "hostname", name)
}

// Sysctl sets a kernel parameter.
func Sysctl(ctx context.Context, key, value string) error {
	return Exec(ctx, "sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
}
