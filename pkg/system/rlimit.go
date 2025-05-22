package system

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"syscall"
)

func Rlimit() error {
	rlimit := syscall.Rlimit{}
	if err := syscall.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("getrlimit error: %v", err)
	}

	logrus.Info("Current Rlimit.Cur: ", rlimit.Cur, ", Rlimit.Max: ", rlimit.Max)

	logrus.Info("Setting Rlimit.Cur to ", rlimit.Max)
	rlimit.Cur = rlimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("failed to set rlimit: %v", err)
	}
	
	if err := syscall.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("getrlimit error: %v", err)
	}
	logrus.Info("Current Rlimit.Cur: ", rlimit.Cur, ", Rlimit.Max: ", rlimit.Max)

	return nil
}
