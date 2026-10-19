// Package supervisor owns a foreground service and its process group.
package supervisor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Config struct {
	Name, Cmd      string
	Args, Env      []string
	Dir            string
	Stdout, Stderr io.Writer
	StopTimeout    time.Duration
}

type Supervisor struct{ cfg Config }

func New(cfg Config) *Supervisor { return &Supervisor{cfg: cfg} }

// Run fails when the service exits unexpectedly. Cancellation sends TERM to
// the whole group, waits a bounded grace period, then kills and reaps it.
func (s *Supervisor) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command(s.cfg.Cmd, s.cfg.Args...)
	cmd.Env = append(os.Environ(), s.cfg.Env...)
	cmd.Dir = s.cfg.Dir
	cmd.Stdout, cmd.Stderr = s.cfg.Stdout, s.cfg.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", s.cfg.Name, err)
	}
	kill := func(sig syscall.Signal) { _ = syscall.Kill(-cmd.Process.Pid, sig) }
	defer kill(syscall.SIGKILL)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return fmt.Errorf("%s service exited unexpectedly", s.cfg.Name)
		}
		return fmt.Errorf("%s service exited: %w", s.cfg.Name, err)
	case <-ctx.Done():
		kill(syscall.SIGTERM)
		grace := s.cfg.StopTimeout
		if grace <= 0 {
			grace = 2 * time.Second
		}
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			kill(syscall.SIGKILL)
			<-done
		}
		return ctx.Err()
	}
}
