package service

import (
	"context"
	"fmt"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/define"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// CreatePodmanInitRCFile 会创建 /etc/init.d/podman，这个 shell script 文件什么都不做，主要是兼容
// https://github.com/oomol/oomol-studio-code/blob/96b3a492f29f581319cfe13c21d5dce400a120ee/oomol-studio-main/desktop/container-server/image/sh/load_images.sh#L17
//
// TODO: 删除这个函数，因为这个函数让人感到困惑
func CreatePodmanInitRCFile() error {
	logrus.Infof("create podman init rc file, but this rc file do nothing just for compatibility")
	err := os.WriteFile("/etc/init.d/podman", []byte(""), 0755)
	if err != nil {
		return fmt.Errorf("failed to create podman init rc file: %s", err)
	}

	return nil
}

func StartGuestPodmanService(ctx context.Context, vmc *define.Machine) error {
	addr := "tcp://" + vmc.PodmanInfo.GuestPodmanAPIListenAddr //nolint:nosprintfhostport

	s := supervisor.New(supervisor.Config{
		Cmd: "podman",
		Args: []string{
			"--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", addr,
		},
		Restart:     true,
		MaxRetries:  5,
		RetryDelay:  500 * time.Millisecond,
		StopTimeout: 5 * time.Second,
	})

	s.Run(ctx)
	return nil
}
