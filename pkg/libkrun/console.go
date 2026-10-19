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
	"unsafe"

	"github.com/creack/pty"
)

// The console carries only kernel and agent diagnostics. Command I/O uses v2.
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
	if err := v.addGuestLogPort(builder, &files); err != nil {
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

type consolePTY struct {
	master *ownedFile
	slave  *ownedFile
}

type libkrunFiles struct {
	consolePTY *consolePTY
	guestLog   *ownedFile
}

func newConsolePTY() (*consolePTY, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	return &consolePTY{master: newOwnedFile(master), slave: newOwnedFile(slave)}, nil
}

func (files *libkrunFiles) startConsoleIO() {
	if files.consolePTY != nil {
		files.consolePTY.start()
	}
}

func (files *libkrunFiles) close() {
	if files.consolePTY != nil {
		files.consolePTY.close()
	}
	closeOwnedFile(files.guestLog)
	*files = libkrunFiles{}
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

func errText(action string) error { return &consoleError{action: action} }

type consoleError struct{ action string }

func (e *consoleError) Error() string { return "libkrun: " + e.action + " failed" }
