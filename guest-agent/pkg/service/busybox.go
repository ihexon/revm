package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// busybox provides access to busybox commands.
type busybox struct {
	path string
}

// Busybox is the global busybox instance. Must call InitializeBusybox first.
var Busybox *busybox

// InitializeBusybox extracts busybox binary and initializes the global instance.
func InitializeBusybox() error {
	path, err := BusyboxBinary.Extract("/.bin")
	if err != nil {
		return err
	}
	Busybox = &busybox{path: path}
	return nil
}

// Exec runs a busybox applet with the given arguments.
// Example: Busybox.Exec(ctx, "mkdir", "-p", "/tmp/foo")
func (b *busybox) Exec(ctx context.Context, args ...string) error {
	if b == nil {
		return fmt.Errorf("busybox not initialized")
	}

	cmd := exec.CommandContext(ctx, b.path, args...)
	cmd.Env = os.Environ()

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr
	}

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Run()
}

// ExecOutput runs a busybox applet and returns the output.
func (b *busybox) ExecOutput(ctx context.Context, args ...string) ([]byte, error) {
	if b == nil {
		return nil, fmt.Errorf("busybox not initialized")
	}

	cmd := exec.CommandContext(ctx, b.path, args...)
	cmd.Env = os.Environ()

	logrus.Debugf("busybox: %v", cmd.Args)
	return cmd.Output()
}

// Path returns the busybox binary path.
func (b *busybox) Path() string {
	if b == nil {
		return ""
	}
	return b.path
}
