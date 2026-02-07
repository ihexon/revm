//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package system

import (
	"fmt"
	"runtime"
	"syscall"

	"github.com/shirou/gopsutil/v4/mem"
	"golang.org/x/sys/unix"
)

func RaiseSystemLimit() error {
	rlimit := syscall.Rlimit{}
	if err := syscall.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return err
	}

	rlimit.Cur = rlimit.Max
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit)
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
