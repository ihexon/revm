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
