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

	fd, err := syscall.Open(devPath, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", devPath, err)
	}

	return os.NewFile(uintptr(fd), devPath), nil
}

// DoExecCmdLine executes the user command and returns its exit code.
// 0 = success, 127 = not found, 126 = not executable.
func DoExecCmdLine(ctx context.Context, vmc *define.Machine) int {
	logrus.Infof("exec: %s %v", vmc.Cmdline.Bin, vmc.Cmdline.Args)

	if err := os.Chdir(vmc.Cmdline.WorkDir); err != nil {
		logrus.Errorf("chdir %q: %v", vmc.Cmdline.WorkDir, err)
		return 1
	}

	cmd := exec.CommandContext(ctx, vmc.Cmdline.Bin, vmc.Cmdline.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if vmc.TTY {
		fdFile, err := openActiveConsole()
		if err != nil {
			logrus.Errorf("open active console: %v", err)
			return 1
		}
		defer fdFile.Close()

		cmd.Stdin = fdFile
		cmd.Stdout = fdFile
		cmd.Stderr = fdFile
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	}

	cmd.Env = append(os.Environ(), vmc.Cmdline.Envs...)

	if err := cmd.Run(); err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			if errors.Is(execErr.Err, exec.ErrNotFound) {
				return 127
			}
			return 126
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		logrus.Errorf("command %q failed: %v", vmc.Cmdline.Bin, err)
		return 1
	}

	return 0
}
