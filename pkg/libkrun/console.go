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
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
)

// setupConsole configures all console ports.
func (v *Libkrun) setupConsole() (retErr error) {
	if ret := C.krun_disable_implicit_console(C.uint32_t(v.ctxID)); ret != 0 {
		return errCode(ret)
	}

	consoleID := C.krun_add_virtio_console_multiport(C.uint32_t(v.ctxID))
	if consoleID < 0 {
		return errCode(consoleID)
	}

	files := libkrunFiles{}
	defer func() {
		if retErr != nil {
			files.close()
		}
	}()

	if err := v.addMainConsole(consoleID, &files); err != nil {
		return err
	}

	if !v.cfg.TTY {
		if err := v.addStdioRedirect(consoleID, &files); err != nil {
			return err
		}
	}

	if err := v.addGuestLogPort(consoleID, &files); err != nil {
		return err
	}

	if err := v.addGuestSignalPort(consoleID, &files); err != nil {
		return err
	}

	v.files = files
	return nil
}

// addMainConsole adds the primary console (hvc0 → /dev/console).
func (v *Libkrun) addMainConsole(consoleID C.int32_t, files *libkrunFiles) (retErr error) {
	if v.cfg.TTY {
		logrus.Info("running in tty mode")
		fd, err := syscall.Dup(int(os.Stdin.Fd()))
		if err != nil {
			return err
		}
		consoleTTY := newOwnedFile(os.NewFile(uintptr(fd), "libkrun-console-tty"))
		defer func() {
			if retErr != nil {
				consoleTTY.close()
			}
		}()

		if err := v.addConsolePortTTY(consoleID, consoleTTY.fd()); err != nil {
			return err
		}
		files.consoleTTY = consoleTTY
		return nil
	}

	logrus.Info("running in non-tty mode")
	consolePTY, err := newConsolePTY()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			consolePTY.close()
		}
	}()

	if err := v.addConsolePortTTY(consoleID, consolePTY.fd()); err != nil {
		return err
	}

	files.consolePTY = consolePTY
	return nil
}

// addStdioRedirect adds stdin/stdout/stderr ports for non-TTY mode.
func (v *Libkrun) addStdioRedirect(consoleID C.int32_t, files *libkrunFiles) (retErr error) {
	pipes, err := newStdioPipes()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			pipes.close()
		}
	}()

	if err := v.addStdioPorts(consoleID, pipes); err != nil {
		return err
	}

	files.stdio = *pipes
	return nil
}

// addGuestLogPort attaches a dedicated guest-log port.
func (v *Libkrun) addGuestLogPort(consoleID C.int32_t, files *libkrunFiles) (retErr error) {
	logFile, err := os.OpenFile(v.cfg.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	guestLog := newOwnedFile(logFile)
	defer func() {
		if retErr != nil {
			guestLog.close()
		}
	}()

	if err := v.addConsolePortInOut(consoleID, consolePortInOut{
		name: define.GuestLogConsolePort,
		in:   -1,
		out:  guestLog.fd(),
	}); err != nil {
		return err
	}

	files.guestLog = guestLog
	return nil
}

// addGuestSignalPort attaches a dedicated guest-signal port.
func (v *Libkrun) addGuestSignalPort(consoleID C.int32_t, files *libkrunFiles) (retErr error) {
	sig, err := newPipeFiles()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			sig.close()
		}
	}()

	if err := v.addConsolePortInOut(consoleID, consolePortInOut{
		name: define.GuestSignalConsolePort,
		in:   sig.read.fd(),
		out:  -1,
	}); err != nil {
		return err
	}

	files.signalPipe = sig
	return nil
}

type consolePortInOut struct {
	name string
	in   int
	out  int
}

type ownedFile struct {
	file *os.File
	once sync.Once
}

type pipeFiles struct {
	read  *ownedFile
	write *ownedFile
}

type stdioPipes struct {
	stdin  pipeFiles
	stdout pipeFiles
	stderr pipeFiles
}

type consolePTY struct {
	master *ownedFile
	slave  *ownedFile
}

type libkrunFiles struct {
	// Keep every *os.File whose fd has been passed to libkrun reachable.
	// libkrun only receives raw fd numbers, so Go's GC cannot see that C code
	// still depends on them. Closing these files early would invalidate the fds
	// that libkrun is using.
	stdio      stdioPipes
	consoleTTY *ownedFile
	consolePTY *consolePTY
	guestLog   *ownedFile

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
	return pipeFiles{read: newOwnedFile(r), write: newOwnedFile(w)}, nil
}

func newConsolePTY() (*consolePTY, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	return &consolePTY{
		master: newOwnedFile(master),
		slave:  newOwnedFile(slave),
	}, nil
}

func (v *Libkrun) addStdioPorts(consoleID C.int32_t, pipes *stdioPipes) error {
	// For pipe-backed stdio, libkrun gets the guest-facing end and Go keeps the
	// host-facing end for forwarding to/from os.Stdin, os.Stdout and os.Stderr.
	ports := []consolePortInOut{
		{name: define.KrunStdinPortName, in: pipes.stdin.read.fd(), out: -1},
		{name: define.KrunStdoutPortName, in: -1, out: pipes.stdout.write.fd()},
		{name: define.KrunStderrPortName, in: -1, out: pipes.stderr.write.fd()},
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

	go copyAndClose(pipes.stdin.write.file, os.Stdin, pipes.stdin.write)
	go copyAndClose(os.Stdout, pipes.stdout.read.file, pipes.stdout.read)
	go copyAndClose(os.Stderr, pipes.stderr.read.file, pipes.stderr.read)
}

func (files *libkrunFiles) startConsoleIO() {
	if files.consolePTY != nil {
		files.consolePTY.start()
	}
	files.stdio.startRedirect()
}

func (files *libkrunFiles) close() {
	files.stdio.close()
	closeOwnedFile(files.consoleTTY)
	if files.consolePTY != nil {
		files.consolePTY.close()
	}
	closeOwnedFile(files.guestLog)
	files.signalPipe.close()
	*files = libkrunFiles{}
}

func (pipes *stdioPipes) close() {
	pipes.stdin.close()
	pipes.stdout.close()
	pipes.stderr.close()
}

func (p pipeFiles) close() {
	closeOwnedFile(p.read)
	closeOwnedFile(p.write)
}

func (p *consolePTY) fd() int {
	return p.slave.fd()
}

func (p *consolePTY) start() {
	go copyOutput(os.Stderr, p.master.file)
}

func (p *consolePTY) close() {
	closeOwnedFile(p.master)
	closeOwnedFile(p.slave)
}

func newOwnedFile(file *os.File) *ownedFile {
	return &ownedFile{file: file}
}

func (f *ownedFile) fd() int {
	return int(f.file.Fd())
}

func (f *ownedFile) close() {
	f.once.Do(func() {
		_ = f.file.Close()
	})
}

func closeOwnedFile(file *ownedFile) {
	if file != nil {
		file.close()
	}
}

func copyOutput(dst io.Writer, src io.Reader) {
	_, _ = io.Copy(dst, src)
}

func copyAndClose(dst io.Writer, src io.Reader, closer *ownedFile) {
	_, _ = io.Copy(dst, src)
	closer.close()
}
