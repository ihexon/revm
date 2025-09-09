package system

import (
	"context"
	"linuxvm/pkg/define"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func StartPodmanService(ctx context.Context) error {
	logrus.Info("start podman API service in guest")
	cmd := exec.CommandContext(ctx, "podman", "system", "service", "--time=0", define.PodmanDefaultListenTcpAddrInGuest)
	logrus.Debugf("podman service cmdline: %q", cmd.Args)
	//if logrus.IsLevelEnabled(logrus.DebugLevel) {
	//	logrus.Debug("enable podman verbose mode")
	//	// TODO: enable podman verbose mode
	//}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil

	return cmd.Run()
}
