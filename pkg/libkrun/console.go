//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <libkrun.h>
*/
import "C"

import (
	"io"
	"linuxvm/pkg/define"
	"os"
	"syscall"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
)

// setupConsole configures all console ports.
func (v *Libkrun) setupConsole() error {
	must(C.krun_disable_implicit_console(C.uint32_t(v.ctxID)))

	consoleID := C.krun_add_virtio_console_multiport(C.uint32_t(v.ctxID))
	if consoleID < 0 {
		return errCode(consoleID)
	}

	if err := v.addMainConsole(consoleID); err != nil {
		return err
	}

	if !v.cfg.TTY {
		if err := v.addStdioRedirect(consoleID); err != nil {
			return err
		}
	}

	if err := v.addGuestLogPort(consoleID); err != nil {
		return err
	}

	return v.addGuestSignalPort(consoleID)
}

// addMainConsole adds the primary console (hvc0 → /dev/console).
func (v *Libkrun) addMainConsole(consoleID C.int32_t) error {
	var fd int

	if v.cfg.TTY {
		logrus.Info("running in tty mode")
		var err error
		fd, err = syscall.Dup(int(os.Stdin.Fd()))
		if err != nil {
			return err
		}
	} else {
		logrus.Info("running in non-tty mode")
		master, slave, err := pty.Open()
		if err != nil {
			return err
		}
		fd = int(slave.Fd())
		v.files.consolePty = [2]*os.File{master, slave}
		go io.Copy(os.Stderr, master)
	}

	name := cstr(define.GuestTTYConsoleName)
	defer free(name)

	ret := C.krun_add_console_port_tty(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(fd),
	)
	if ret != 0 {
		if v.cfg.TTY {
			syscall.Close(fd)
		}
		return errCode(ret)
	}
	return nil
}

// addStdioRedirect adds stdin/stdout/stderr ports for non-TTY mode.
func (v *Libkrun) addStdioRedirect(consoleID C.int32_t) error {
	stdinR, stdinW := pipe()
	go func() {
		io.Copy(stdinW, os.Stdin)
		stdinW.Close()
	}()

	stdoutR, stdoutW := pipe()
	go io.Copy(os.Stdout, stdoutR)

	stderrR, stderrW := pipe()
	go io.Copy(os.Stderr, stderrR)

	ports := []struct {
		name string
		in   int
		out  int
	}{
		{define.KrunStdinPortName, int(stdinR.Fd()), -1},
		{define.KrunStdoutPortName, -1, int(stdoutW.Fd())},
		{define.KrunStderrPortName, -1, int(stderrW.Fd())},
	}

	for _, p := range ports {
		name := cstr(p.name)
		ret := C.krun_add_console_port_inout(
			C.uint32_t(v.ctxID),
			C.uint32_t(consoleID),
			name,
			C.int(p.in),
			C.int(p.out),
		)
		free(name)
		if ret != 0 {
			return errCode(ret)
		}
	}

	v.files.stdin = stdinR
	v.files.stdout = stdoutW
	v.files.stderr = stderrW
	return nil
}

// addGuestLogPort attaches a dedicated guest-log port.
func (v *Libkrun) addGuestLogPort(consoleID C.int32_t) error {
	logFile, err := os.OpenFile(v.cfg.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	name := cstr(define.GuestLogConsolePort)
	defer free(name)

	ret := C.krun_add_console_port_inout(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(-1),
		C.int(logFile.Fd()),
	)
	if ret != 0 {
		_ = logFile.Close()
		return errCode(ret)
	}

	v.files.guestLog = logFile
	return nil
}

// addGuestSignalPort attaches a dedicated guest-signal port.
func (v *Libkrun) addGuestSignalPort(consoleID C.int32_t) error {
	sigR, sigW := pipe()

	name := cstr(define.GuestSignalConsolePort)
	defer free(name)

	ret := C.krun_add_console_port_inout(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(sigR.Fd()),
		C.int(-1),
	)
	if ret != 0 {
		_ = sigR.Close()
		_ = sigW.Close()
		return errCode(ret)
	}

	v.files.signalPipeR = sigR
	v.files.signalPipeW = sigW
	return nil
}

func pipe() (*os.File, *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	return r, w
}
