//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package libkrun

/*
#cgo CFLAGS: -I ../../include
#include <libkrun.h>
*/
import "C"

import (
	"errors"
	"io"
	"linuxvm/pkg/define"
	"os"
	"unsafe"

	"github.com/creack/pty"
)

// Keep the descriptors alive until process exit. The console carries only
// kernel and agent diagnostics; command I/O uses the execution connection.
type libkrunFiles struct {
	master, slave, guestLog *os.File
}

func (v *machine) setupConsole() error {
	builder := C.krun_console_device_builder()
	if builder == nil {
		return errors.New("libkrun: create console builder failed")
	}
	var err error
	v.files.master, v.files.slave, err = pty.Open()
	if err != nil {
		return err
	}
	v.files.guestLog, err = os.OpenFile(v.cfg.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	name := newKrunStr(define.GuestTTYConsoleName)
	defer name.free()
	var port C.uint32_t
	var errOut C.KrunError
	result := C.krun_console_builder_add_tty_port(builder, name.value, C.int(v.files.slave.Fd()), &port, &errOut)
	if err = checkKrunResult("add console TTY port", result, errOut); err != nil {
		return err
	}
	logName := newKrunStr(define.GuestLogConsolePort)
	defer logName.free()
	errOut = nil
	result = C.krun_console_builder_add_inout_port(builder, logName.value, -1, C.int(v.files.guestLog.Fd()), &port, &errOut)
	if err = checkKrunResult("add guest log port", result, errOut); err != nil {
		return err
	}
	errOut = nil
	console := C.krun_console_builder_build(builder, &errOut)
	if err = checkKrunHandle("build console", unsafe.Pointer(console), errOut); err != nil {
		return err
	}
	C.krun_mmio_device_manager_add(v.manager, C.KrunAttachDevice(unsafe.Pointer(console)))
	return nil
}

func (files *libkrunFiles) startConsoleIO() {
	go func() { _, _ = io.Copy(files.guestLog, files.master) }()
}
