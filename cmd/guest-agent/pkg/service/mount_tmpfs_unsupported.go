//go:build !linux

package service

import "fmt"

func mountTmpfs(target, options string) error {
	return fmt.Errorf("tmpfs mount is only supported on linux")
}
