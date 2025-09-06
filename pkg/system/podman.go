package system

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func StartPodmanService(ctx context.Context) error {
	logrus.Info("start podman API service")
	// podman system service --time=0 tcp://192.168.127.2:25883
	cmd := exec.CommandContext(ctx, "podman", "system", "service", "--time=0", define.PodmanDefaultListenTcpAddrInGuest)
	logrus.Infof("podman service cmdline: %q", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
