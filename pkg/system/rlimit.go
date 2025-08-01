package system

import (
	"fmt"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"runtime"
	"syscall"
)

func Rlimit() error {
	rlimit := syscall.Rlimit{}
	if err := syscall.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("getrlimit error: %v", err)
	}
	logrus.Info("Current Rlimit.Cur: ", rlimit.Cur, ", Rlimit.Max: ", rlimit.Max)

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

func GetCPUCores() int {
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}

	return cores
}

func GetMaxMemoryInMB() (uint64, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, fmt.Errorf("get virtual memory error: %w", err)
	}

	mb := vmStat.Total / 1024 / 1024
	return mb, nil
}
