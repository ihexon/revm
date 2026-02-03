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
	logrus.Infof("exec: %s %v", vmc.Cmdline.Bin, vmc.Cmdline.Args)

	if err := os.Chdir(vmc.Cmdline.WorkDir); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, vmc.Cmdline.Bin, vmc.Cmdline.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), vmc.Cmdline.Envs...)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", vmc.Cmdline.Bin, err)
	}

	return ErrProcessExitNormal
}
