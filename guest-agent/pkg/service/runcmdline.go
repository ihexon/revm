package service

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

var ErrProcessExitNormal = errors.New("process exit normally")

func DoExecCmdLine(ctx context.Context, vmc *define.VMConfig) error {
	cmd := exec.CommandContext(ctx, vmc.Cmdline.Bin, vmc.Cmdline.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), vmc.Cmdline.Envs...)

	logrus.Debugf("full cmdline: %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cmdline %q exit with err: %w", cmd.Args, err)
	}

	return ErrProcessExitNormal
}
