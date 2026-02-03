package service

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

type busybox struct {
	path string
}

var Busybox *busybox

func InitializeBusybox() error {
	logrus.Debug("initializing busybox...")
	path, err := BusyboxBinary.ExtractToDir(define.GuestHiddenBinDir)
	if err != nil {
		return err
	}
	Busybox = &busybox{path: path}
	logrus.Debugf("busybox initialized at %q", path)
	return nil
}

func (b *busybox) Exec(ctx context.Context, args ...string) error {
	if b == nil {
		return fmt.Errorf("busybox not initialized")
	}

	cmd := exec.CommandContext(ctx, b.path, args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Run()
}

func (b *busybox) ExecQuiet(ctx context.Context, args ...string) error {
	if b == nil {
		return fmt.Errorf("busybox not initialized")
	}

	cmd := exec.CommandContext(ctx, b.path, args...)
	cmd.Env = os.Environ()

	cmd.Stderr = os.Stderr
	cmd.Stdout = nil
	cmd.Stdin = nil

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Run()
}
