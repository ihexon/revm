package service

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func ExecNoOutput(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, BusyboxPath(), args...)
	cmd.Env = os.Environ()
	cmd.Stderr = nil
	cmd.Stdout = nil

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
	return ExecNoOutput(ctx, append([]string{"mount"}, args...)...)
}

// Umount runs busybox umount command.
func Umount(ctx context.Context, target string) error {
	return ExecNoOutput(ctx, "umount", "-l", "-d", "-f", target)
}

// IsMounted checks if a path is a mount point.
func IsMounted(target string) bool {
	if err := ExecNoOutput(context.Background(), "mountpoint", "-q", target); err != nil {
		return false
	}
	return true
}