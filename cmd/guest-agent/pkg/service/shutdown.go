package service

import (
	"context"
	"os/exec"

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

// 暂时不调用
func Shutdown(ctx context.Context) {
	<-ctx.Done()

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
	logrus.Infof("poweroff machine...")
	stepS.PowerOff()
	return
}
