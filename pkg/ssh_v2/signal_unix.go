//go:build !windows

package ssh_v2

import (
	"os"
	"os/signal"
	"syscall"
)

func signalNotify(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}

func signalStop(ch chan<- os.Signal) {
	signal.Stop(ch)
}
