package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/moby/sys/mountinfo"
	"github.com/sirupsen/logrus"
)

func ExecNoOutput(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("command is empty")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stderr = nil
	cmd.Stdout = nil

	logrus.Debugf("guest command: %v", cmd.Args)
	return cmd.Run()
}

// ExecOutput runs a rootfs command and captures output to the provided writers.
func ExecOutput(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("command is empty")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	logrus.Debugf("guest command: %v", cmd.Args)
	return cmd.Run()
}

// Mount runs the rootfs mount command.
func Mount(ctx context.Context, args ...string) error {
	return ExecOutput(ctx, StderrWriter(), StderrWriter(), append([]string{"mount"}, args...)...)
}

// Umount runs the rootfs umount command.
func Umount(ctx context.Context, target string) error {
	return ExecNoOutput(ctx, "umount", "-l", "-d", "-f", target)
}

// IsMounted checks if a path is a mount point.
func IsMounted(target string) bool {
	mounted, err := mountinfo.Mounted(target)
	if err != nil {
		logrus.Debugf("mountinfo: %s: %v", target, err)
		return false
	}
	return mounted
}
