//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#include <libkrun.h>
*/
import "C"

import (
	"encoding/json"
	"io"
	"linuxvm/pkg/define"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
)

// setupConsole configures all console ports.
func (v *VM) setupConsole() error {
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

	return v.addLogAndSignal(consoleID)
}

// addMainConsole adds the primary console (hvc0 → /dev/console).
func (v *VM) addMainConsole(consoleID C.int32_t) error {
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
func (v *VM) addStdioRedirect(consoleID C.int32_t) error {
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

// addLogAndSignal adds the guest-logs port with signal forwarding.
func (v *VM) addLogAndSignal(consoleID C.int32_t) error {
	sigR, sigW := pipe()
	v.files.signalPipe = sigW

	// Forward host signals to guest
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer signal.Stop(ch)

		for sig := range ch {
			msg := struct{ SignalName string }{SignalName: sig.String()}
			if b, err := json.Marshal(msg); err == nil {
				if _, err := sigW.Write(b); err != nil {
					return
				}
				sigW.WriteString("\n")
			}
		}
	}()

	logFile, err := os.OpenFile(v.cfg.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	v.files.guestLog = logFile

	name := cstr(define.GuestLogConsolePort)
	defer free(name)

	ret := C.krun_add_console_port_inout(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(sigR.Fd()),
		C.int(logFile.Fd()),
	)
	if ret != 0 {
		logFile.Close()
		return errCode(ret)
	}
	return nil
}

func pipe() (*os.File, *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	return r, w
}
