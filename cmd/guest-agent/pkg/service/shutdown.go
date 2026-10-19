package service

import (
	"context"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// RunGuest owns all services. A required service failure shuts down the VM.
// Every shutdown shares a five-second budget, including command/container
// cleanup and disk sync. The host remains the final force-stop boundary.
func RunGuest(ctx context.Context, vmc *protocol.GuestSpec) error {
	g, services := errgroup.WithContext(ctx)
	container := vmc.RunMode == define.ContainerMode.String()
	if !container && vmc.RunMode != define.RootFsMode.String() {
		return fmt.Errorf("unsupported mode %q", vmc.RunMode)
	}
	g.Go(func() error { return StartGuestControlServer(services, container) })
	g.Go(func() error { return SyncRTCTime(services) })
	if container {
		g.Go(func() error { return StartGuestPodmanService(services, vmc) })
	}
	done := make(chan error, 1)
	go func() { done <- g.Wait() }()
	<-services.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	select {
	case err = <-done:
		if errors.Is(err, context.Canceled) {
			err = nil
		}
	case <-shutdown.Done():
		err = errors.New("guest shutdown deadline exceeded")
	}
	if err != nil {
		logrus.Error(err)
	}
	logrus.Info("guest services stopped; syncing disks")
	if syncErr := ExecNoOutput(shutdown, "sync"); syncErr != nil {
		logrus.Errorf("sync disks: %v", syncErr)
		err = errors.Join(err, syncErr)
	}
	if err != nil {
		return err
	} // Let init observe service failure instead of a successful reboot.
	logrus.Info("rebooting virtual machine")
	return errors.Join(err, syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART))
}
