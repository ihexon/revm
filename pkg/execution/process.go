//go:build linux || darwin

package execution

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/creack/pty"
)

type noClose struct{ io.Writer }

func (noClose) Close() error { return nil }

type emptyOutput struct{ io.Reader }

func (emptyOutput) Close() error { return nil }

func startProcess(req Request) (cmd *exec.Cmd, input io.WriteCloser, stdout, stderr io.ReadCloser, terminal *os.File, owned *boundary, err error) {
	cmd = exec.Command(req.Argv[0], req.Argv[1:]...)
	cmd.Env = append(os.Environ(), req.Env...)
	cmd.Dir = req.WorkDir
	if cmd.Dir == "" {
		cmd.Dir = "/"
	}
	stdout = emptyOutput{strings.NewReader("")}
	stderr = emptyOutput{strings.NewReader("")}
	input = noClose{io.Discard}
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if !req.TTY {
		cmd.SysProcAttr.Setpgid = true
	}
	owned, err = prepareBoundary(cmd)
	if err != nil {
		return
	}
	if req.TTY {
		if req.Term == "" {
			req.Term = "xterm-256color"
		}
		cmd.Env = append(cmd.Env, "TERM="+req.Term)
		if req.Rows == 0 {
			req.Rows = 24
		}
		if req.Cols == 0 {
			req.Cols = 80
		}
		terminal, err = pty.StartWithSize(cmd, &pty.Winsize{Rows: req.Rows, Cols: req.Cols})
		if err == nil {
			input = noClose{terminal}
			stdout = terminal
		}
		return
	}
	input, err = cmd.StdinPipe()
	if err != nil {
		input = noClose{io.Discard}
		return
	}
	outR, outW, e := os.Pipe()
	if e != nil {
		err = e
		return
	}
	stdout = outR
	defer outW.Close()
	errR, errW, e := os.Pipe()
	if e != nil {
		err = e
		return
	}
	stderr = errR
	defer errW.Close()
	cmd.Stdout, cmd.Stderr = outW, errW
	err = cmd.Start()
	return
}
