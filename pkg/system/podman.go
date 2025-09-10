package system

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func StartPodmanService(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "podman", "system", "service", "--time=0", define.PodmanDefaultListenTcpAddrInGuest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil

	logrus.Debugf("podman service cmdline: %q", cmd.Args)
	return cmd.Run()
}
