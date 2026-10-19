//go:build linux

package execution

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var guestIsolation bool

func RequireGuestIsolation() { guestIsolation = true }

type boundary struct {
	cmd *exec.Cmd
	dir string
	fd  *os.File
}

func prepareBoundary(cmd *exec.Cmd) (*boundary, error) {
	b := &boundary{cmd: cmd}
	if !guestIsolation {
		return b, nil
	}
	base := "/sys/fs/cgroup/revm-exec"
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, fmt.Errorf("execution cgroup: %w", err)
	}
	dir, err := os.MkdirTemp(base, "exec-")
	if err != nil {
		return nil, err
	}
	b.dir = dir
	b.fd, err = os.Open(dir)
	if err != nil {
		os.Remove(dir)
		return nil, err
	}
	cmd.SysProcAttr.UseCgroupFD = true
	cmd.SysProcAttr.CgroupFD = int(b.fd.Fd())
	cmd.SysProcAttr.Cloneflags = unix.CLONE_NEWNS | unix.CLONE_NEWCGROUP
	exe, err := os.Executable()
	if err != nil {
		b.Close()
		return nil, err
	}
	cmd.Args = append([]string{exe, "_exec", dir}, cmd.Args...)
	cmd.Path = exe
	return b, nil
}
func (b *boundary) Signal(sig syscall.Signal) {
	if b.dir != "" {
		data, _ := os.ReadFile(filepath.Join(b.dir, "cgroup.procs"))
		for _, s := range strings.Fields(string(data)) {
			pid, e := strconv.Atoi(s)
			if e == nil {
				_ = syscall.Kill(pid, sig)
			}
		}
		return
	}
	if b.cmd.Process != nil {
		_ = syscall.Kill(-b.cmd.Process.Pid, sig)
	}
}
func (b *boundary) Kill() error {
	if b.dir == "" {
		b.Signal(syscall.SIGKILL)
		return nil
	}
	return os.WriteFile(filepath.Join(b.dir, "cgroup.kill"), []byte("1"), 0600)
}
func (b *boundary) Close() error {
	if b.fd != nil {
		b.fd.Close()
		b.fd = nil
	}
	if b.dir == "" {
		return nil
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := os.Remove(b.dir)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("execution cgroup cleanup: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ExecIsolated runs after clone3 atomically joins the execution cgroup. A private
// cgroup mount and reduced privilege prevent commands moving outside that group.
func ExecIsolated(argv []string) error {
	if len(argv) < 2 {
		return fmt.Errorf("missing execution command")
	}
	group, argv := argv[0], argv[1:]
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("private mounts: %w", err)
	}
	// Expose only this execution's subtree. A bind mount also works when the
	// kernel refuses a second cgroup2 superblock mount in a cgroup namespace.
	if err := unix.Mount(group, "/sys/fs/cgroup", "", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("private cgroup mount: %w", err)
	}
	if err := unix.Mount("", "/sys/fs/cgroup", "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("read-only cgroup mount: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	data := [2]unix.CapUserData{}
	if err := unix.Capget(&header, &data[0]); err != nil {
		return err
	}
	for _, cap := range []int{unix.CAP_SYS_ADMIN, unix.CAP_SYS_PTRACE, unix.CAP_DAC_READ_SEARCH} {
		if err := unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(cap), 0, 0, 0); err != nil {
			return err
		}
		mask := uint32(1) << uint(cap%32)
		data[cap/32].Effective &^= mask
		data[cap/32].Permitted &^= mask
		data[cap/32].Inheritable &^= mask
	}
	if err := unix.Capset(&header, &data[0]); err != nil {
		return err
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}
