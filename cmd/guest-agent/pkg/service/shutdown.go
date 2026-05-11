package service

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

type Step struct {
	diskSync chan bool
}

func (s *Step) SyncDisk() {
	_ = exec.Command("sync").Run()
	close(s.diskSync)
}

func (s *Step) PowerOff() {
	_ = exec.Command("poweroff", "-f").Run()
}

// WaitAndShutdown waits for the interrupt signal and shutdown the VM
// this is the only way to shutdown the VM gracefully
//
// the latest libkrun explicitly issue sync+reboot;
// theoretically, manual sync operations are no longer required.
// See: https://github.com/containers/libkrun/commit/5cfd94dafd739a880f67da13d1483880c8d86027
func WaitAndShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	stepS := &Step{
		diskSync: make(chan bool),
	}

	go func() {
		stepS.SyncDisk()
	}()

	logrus.Infof("[shutdown] waiting for disk to sync...")
	<-stepS.diskSync

	logrus.Infof("[shutdown] poweroff virtual machine...")
	stepS.PowerOff()
}
