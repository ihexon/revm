package service

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

type Step struct {
	podmanStop chan bool
	diskSync   chan bool
}

func (s *Step) PodmanStop() {
	_ = exec.Command("podman", "stop", "-a", "-t", "1").Run()
	s.podmanStop <- true
	close(s.podmanStop)
}

func (s *Step) SyncDisk() {

	_ = exec.Command("sync").Run()
	s.diskSync <- true
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
		podmanStop: make(chan bool),
		diskSync:   make(chan bool),
	}

	go func() {
		logrus.Infof("Waiting for podman to stop...")
		stepS.PodmanStop()
	}()
	go func() {
		logrus.Infof("Waiting for disk to sync...")
		stepS.SyncDisk()
	}()

	<-stepS.podmanStop
	<-stepS.diskSync

	stepS.PowerOff()
}
