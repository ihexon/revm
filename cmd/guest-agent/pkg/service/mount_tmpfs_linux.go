//go:build linux

package service

import "syscall"

func mountTmpfs(target, options string) error {
	return syscall.Mount("tmpfs", target, "tmpfs", 0, options)
}
