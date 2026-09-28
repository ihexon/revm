//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"io"
	"linuxvm/pkg/define"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
)

// setupConsole builds the virtio-console device with the same named ports
// used by the guest agent in the 1.x integration.
func (v *Libkrun) setupConsole() (retErr error) {
	builder := C.krun_console_device_builder()
	if builder == nil {
		return errText("create console builder")
	}
	defer func() {
		if builder != nil {
			C.krun_console_builder_destroy(builder)
		}
	}()

	files := libkrunFiles{}
	defer func() {
		if retErr != nil {
			files.close()
		}
	}()
	if err := v.addMainConsole(builder, &files); err != nil {
		return err
	}
	if !v.cfg.TTY {
		if err := v.addStdioRedirect(builder, &files); err != nil {
			return err
		}
	}
	if err := v.addGuestLogPort(builder, &files); err != nil {
		return err
	}
	if err := v.addGuestSignalPort(builder, &files); err != nil {
		return err
	}

	var errOut C.KrunError
	v.console = C.krun_console_builder_build(builder, &errOut)
	builder = nil
	if err := checkKrunHandle("build console", unsafe.Pointer(v.console), errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(v.console)))
	v.console = nil
	v.files = files
	return nil
}

func (v *Libkrun) addMainConsole(builder C.KrunConsoleBuilder, files *libkrunFiles) (retErr error) {
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
		if err := addConsoleTTY(builder, define.GuestTTYConsoleName, consoleTTY.fd()); err != nil {
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
	if err := addConsoleTTY(builder, define.GuestTTYConsoleName, consolePTY.fd()); err != nil {
		return err
	}
	files.consolePTY = consolePTY
	return nil
}

func (v *Libkrun) addStdioRedirect(builder C.KrunConsoleBuilder, files *libkrunFiles) (retErr error) {
	pipes, err := newStdioPipes()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			pipes.close()
		}
	}()
	for _, port := range []consolePortInOut{
		{name: define.KrunStdinPortName, in: pipes.stdin.read.fd(), out: -1},
		{name: define.KrunStdoutPortName, in: -1, out: pipes.stdout.write.fd()},
		{name: define.KrunStderrPortName, in: -1, out: pipes.stderr.write.fd()},
	} {
		if err := addConsoleInOut(builder, port); err != nil {
			return err
		}
	}
	files.stdio = *pipes
	return nil
}

func (v *Libkrun) addGuestLogPort(builder C.KrunConsoleBuilder, files *libkrunFiles) (retErr error) {
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
	if err := addConsoleInOut(builder, consolePortInOut{name: define.GuestLogConsolePort, in: -1, out: guestLog.fd()}); err != nil {
		return err
	}
	files.guestLog = guestLog
	return nil
}

func (v *Libkrun) addGuestSignalPort(builder C.KrunConsoleBuilder, files *libkrunFiles) (retErr error) {
	sig, err := newPipeFiles()
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			sig.close()
		}
	}()
	if err := addConsoleInOut(builder, consolePortInOut{name: define.GuestSignalConsolePort, in: sig.read.fd(), out: -1}); err != nil {
		return err
	}
	files.signalPipe = sig
	return nil
}

func addConsoleTTY(builder C.KrunConsoleBuilder, name string, fd int) error {
	n := newKrunStr(name)
	defer n.free()
	var port C.uint32_t
	var errOut C.KrunError
	result := C.krun_console_builder_add_tty_port(builder, n.value, C.int(fd), &port, &errOut)
	return checkKrunResult("add console TTY port", result, errOut)
}

type consolePortInOut struct {
	name string
	in   int
	out  int
}

func addConsoleInOut(builder C.KrunConsoleBuilder, port consolePortInOut) error {
	n := newKrunStr(port.name)
	defer n.free()
	var resultIndex C.uint32_t
	var errOut C.KrunError
	result := C.krun_console_builder_add_inout_port(builder, n.value, C.int(port.in), C.int(port.out), &resultIndex, &errOut)
	return checkKrunResult("add console I/O port", result, errOut)
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
	stdio      stdioPipes
	consoleTTY *ownedFile
	consolePTY *consolePTY
	guestLog   *ownedFile
	signalPipe pipeFiles
}

func newStdioPipes() (_ *stdioPipes, retErr error) {
	pipes := &stdioPipes{}
	defer func() {
		if retErr != nil {
			pipes.close()
		}
	}()
	for _, dst := range []*pipeFiles{&pipes.stdin, &pipes.stdout, &pipes.stderr} {
		p, err := newPipeFiles()
		if err != nil {
			return nil, err
		}
		*dst = p
	}
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
	return &consolePTY{master: newOwnedFile(master), slave: newOwnedFile(slave)}, nil
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

func (p *consolePTY) fd() int { return p.slave.fd() }

func (p *consolePTY) start() { go copyOutput(os.Stderr, p.master.file) }

func (p *consolePTY) close() {
	closeOwnedFile(p.master)
	closeOwnedFile(p.slave)
}

func newOwnedFile(file *os.File) *ownedFile { return &ownedFile{file: file} }

func (f *ownedFile) fd() int { return int(f.file.Fd()) }

func (f *ownedFile) close() { f.once.Do(func() { _ = f.file.Close() }) }

func closeOwnedFile(file *ownedFile) {
	if file != nil {
		file.close()
	}
}

func copyOutput(dst io.Writer, src io.Reader) { _, _ = io.Copy(dst, src) }

func copyAndClose(dst io.Writer, src io.Reader, closer *ownedFile) {
	_, _ = io.Copy(dst, src)
	closer.close()
}

func errText(action string) error { return &consoleError{action: action} }

type consoleError struct{ action string }

func (e *consoleError) Error() string { return "libkrun: " + e.action + " failed" }
