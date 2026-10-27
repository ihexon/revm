package execution

import (
	"os/exec"
	"syscall"
)

// Host protocol tests use process groups. Production execution is always in
// the Linux guest, where RequireGuestIsolation makes cgroups mandatory.
type boundary struct{ cmd *exec.Cmd }

func prepareBoundary(cmd *exec.Cmd) (*boundary, error) { return &boundary{cmd: cmd}, nil }
func (b *boundary) Kill() error {
	if b.cmd.Process != nil {
		_ = syscall.Kill(-b.cmd.Process.Pid, syscall.SIGKILL)
	}
	return nil
}
func (b *boundary) Close() error { return nil }
