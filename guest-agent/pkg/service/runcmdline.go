package service

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

var ErrProcessExitNormal = errors.New("process exit normally")

// activeConsoleTTY reads /sys/class/tty/console/active to find the real
// TTY device backing the kernel console. The kernel gives init fd 0/1/2
// pointing to /dev/console (major 5:1), a redirect device that does NOT
// support TTY ioctls (TIOCGWINSZ), causing isatty() to fail.
// Opening the underlying device (e.g. /dev/hvc0, /dev/ttyS0) directly
// gives a proper TTY.
//
// When multiple console= params exist, the file contains space-separated
// names and the last one is the primary console.
func activeConsoleTTY() (string, error) {
	data, err := os.ReadFile("/sys/class/tty/console/active")
	if err != nil {
		return "", fmt.Errorf("read active console: %w", err)
	}

	names := strings.Fields(strings.TrimSpace(string(data)))
	if len(names) == 0 {
		return "", fmt.Errorf("no active console found")
	}

	return "/dev/" + names[len(names)-1], nil
}

func openActiveConsole() (*os.File, error) {
	devPath, err := activeConsoleTTY()
	if err != nil {
		return nil, err
	}

	fd, err := syscall.Open(devPath, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", devPath, err)
	}

	return os.NewFile(uintptr(fd), devPath), nil
}

func DoExecCmdLine(ctx context.Context, vmc *define.VMConfig) error {
	logrus.Infof("exec: %s %v", vmc.Cmdline.Bin, vmc.Cmdline.Args)

	if err := os.Chdir(vmc.Cmdline.WorkDir); err != nil {
		return err
	}

	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr

	cmd := exec.CommandContext(ctx, vmc.Cmdline.Bin, vmc.Cmdline.Args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), vmc.Cmdline.Envs...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", vmc.Cmdline.Bin, err)
	}

	return ErrProcessExitNormal
}
