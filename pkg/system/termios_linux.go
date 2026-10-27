//go:build linux

package system

import "golang.org/x/sys/unix"

func SaveTerminal(fd int) *unix.Termios {
	state, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil
	}
	return state
}
func RestoreTerminal(fd int, state *unix.Termios) error {
	if state != nil {
		return unix.IoctlSetTermios(fd, unix.TCSETS, state)
	}
	return nil
}
