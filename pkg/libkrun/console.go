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
	if ret := C.krun_disable_implicit_console(C.uint32_t(v.ctxID)); ret != 0 {
		return errCode(ret)
	}

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
	if v.cfg.TTY {
		logrus.Info("running in tty mode")
		fd, err := syscall.Dup(int(os.Stdin.Fd()))
		if err != nil {
			return err
		}

		if err := v.addConsolePortTTY(consoleID, fd); err != nil {
			_ = syscall.Close(fd)
			return err
		}
		v.files.consoleTTY = os.NewFile(uintptr(fd), "libkrun-console-tty")
		return nil
	}

	logrus.Info("running in non-tty mode")
	consolePTY, err := newConsolePTY()
	if err != nil {
		return err
	}

	if err := v.addConsolePortTTY(consoleID, consolePTY.fd()); err != nil {
		consolePTY.close()
		return err
	}

	v.files.consolePTY = consolePTY
	return nil
}

// addStdioRedirect adds stdin/stdout/stderr ports for non-TTY mode.
func (v *Libkrun) addStdioRedirect(consoleID C.int32_t) error {
	pipes, err := newStdioPipes()
	if err != nil {
		return err
	}

	if err := v.addStdioPorts(consoleID, pipes); err != nil {
		pipes.close()
		return err
	}

	v.files.stdio = *pipes
	return nil
}

// addGuestLogPort attaches a dedicated guest-log port.
func (v *Libkrun) addGuestLogPort(consoleID C.int32_t) error {
	logFile, err := os.OpenFile(v.cfg.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	if err := v.addConsolePortInOut(consoleID, consolePortInOut{
		name: define.GuestLogConsolePort,
		in:   -1,
		out:  int(logFile.Fd()),
	}); err != nil {
		_ = logFile.Close()
		return err
	}

	v.files.guestLog = logFile
	return nil
}

// addGuestSignalPort attaches a dedicated guest-signal port.
func (v *Libkrun) addGuestSignalPort(consoleID C.int32_t) error {
	sig, err := newPipeFiles()
	if err != nil {
		return err
	}

	if err := v.addConsolePortInOut(consoleID, consolePortInOut{
		name: define.GuestSignalConsolePort,
		in:   int(sig.read.Fd()),
		out:  -1,
	}); err != nil {
		sig.close()
		return err
	}

	v.files.signalPipe = sig
	return nil
}

type consolePortInOut struct {
	name string
	in   int
	out  int
}

type pipeFiles struct {
	read  *os.File
	write *os.File
}

type stdioPipes struct {
	stdin  pipeFiles
	stdout pipeFiles
	stderr pipeFiles
}

type consolePTY struct {
	master *os.File
	slave  *os.File
}

type libkrunFiles struct {
	stdio      stdioPipes
	consoleTTY *os.File
	consolePTY *consolePTY
	guestLog   *os.File

	// Keep the read end alive for Libkrun and the write end for guest signals.
	signalPipe pipeFiles
}

func newStdioPipes() (_ *stdioPipes, retErr error) {
	pipes := &stdioPipes{}
	defer func() {
		if retErr != nil {
			pipes.close()
		}
	}()

	stdin, err := newPipeFiles()
	if err != nil {
		return nil, err
	}
	pipes.stdin = stdin

	stdout, err := newPipeFiles()
	if err != nil {
		return nil, err
	}
	pipes.stdout = stdout

	stderr, err := newPipeFiles()
	if err != nil {
		return nil, err
	}
	pipes.stderr = stderr

	return pipes, nil
}

func newPipeFiles() (pipeFiles, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return pipeFiles{}, err
	}
	return pipeFiles{read: r, write: w}, nil
}

func newConsolePTY() (*consolePTY, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	return &consolePTY{master: master, slave: slave}, nil
}

func (v *Libkrun) addStdioPorts(consoleID C.int32_t, pipes *stdioPipes) error {
	ports := []consolePortInOut{
		{name: define.KrunStdinPortName, in: int(pipes.stdin.read.Fd()), out: -1},
		{name: define.KrunStdoutPortName, in: -1, out: int(pipes.stdout.write.Fd())},
		{name: define.KrunStderrPortName, in: -1, out: int(pipes.stderr.write.Fd())},
	}

	for _, port := range ports {
		if err := v.addConsolePortInOut(consoleID, port); err != nil {
			return err
		}
	}
	return nil
}

func (v *Libkrun) addConsolePortTTY(consoleID C.int32_t, fd int) error {
	name := cstr(define.GuestTTYConsoleName)
	defer free(name)

	ret := C.krun_add_console_port_tty(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(fd),
	)
	if ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (v *Libkrun) addConsolePortInOut(consoleID C.int32_t, port consolePortInOut) error {
	name := cstr(port.name)
	defer free(name)

	ret := C.krun_add_console_port_inout(
		C.uint32_t(v.ctxID),
		C.uint32_t(consoleID),
		name,
		C.int(port.in),
		C.int(port.out),
	)
	if ret != 0 {
		return errCode(ret)
	}
	return nil
}

func (pipes *stdioPipes) startRedirect() {
	if pipes.stdin.write == nil {
		return
	}

	go copyAndClose(pipes.stdin.write, os.Stdin, pipes.stdin.write)
	go copyAndClose(os.Stdout, pipes.stdout.read, pipes.stdout.read)
	go copyAndClose(os.Stderr, pipes.stderr.read, pipes.stderr.read)
}

func (files *libkrunFiles) startConsoleIO() {
	if files.consolePTY != nil {
		files.consolePTY.start()
	}
	files.stdio.startRedirect()
}

func (files *libkrunFiles) close() {
	files.stdio.close()
	closeFile(files.consoleTTY)
	if files.consolePTY != nil {
		files.consolePTY.close()
	}
	closeFile(files.guestLog)
	files.signalPipe.close()
	*files = libkrunFiles{}
}

func (pipes *stdioPipes) close() {
	pipes.stdin.close()
	pipes.stdout.close()
	pipes.stderr.close()
}

func (p pipeFiles) close() {
	closeFile(p.read)
	closeFile(p.write)
}

func (p *consolePTY) fd() int {
	return int(p.slave.Fd())
}

func (p *consolePTY) start() {
	go copyOutput(os.Stderr, p.master)
}

func (p *consolePTY) close() {
	closeFile(p.master)
	closeFile(p.slave)
}

func closeFile(file *os.File) {
	if file != nil {
		_ = file.Close()
	}
}

func copyOutput(dst io.Writer, src io.Reader) {
	_, _ = io.Copy(dst, src)
}

func copyAndClose(dst io.Writer, src io.Reader, closer io.Closer) {
	_, _ = io.Copy(dst, src)
	_ = closer.Close()
}
