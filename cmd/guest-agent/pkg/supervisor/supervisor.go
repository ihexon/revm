package supervisor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type Config struct {
	Name string // label for log messages
	Cmd  string
	Args []string
	Env  []string
	Dir  string

	Stdout io.Writer
	Stderr io.Writer

	Restart    bool
	MaxRetries int           // 最大重启次数（0 = 无限）
	RetryDelay time.Duration // 重启间隔

	StopTimeout time.Duration
}

type Supervisor struct {
	cfg Config

	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan struct{}
	stopping bool
	running  bool
	restarts int
}

func New(cfg Config) *Supervisor {
	return &Supervisor{
		cfg: cfg,
	}
}

// Run starts the supervisor and blocks until it stops.
func (s *Supervisor) Run(ctx context.Context) {
	s.loop(ctx)
}

func (s *Supervisor) loop(ctx context.Context) {
	for {
		if s.isStopping() {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.runOnce(ctx)
		if err != nil {
			logrus.Infof("[supervisor:%s] process exited: %s", s.cfg.Name, err)
		}

		if !s.cfg.Restart || s.isStopping() {
			return
		}

		s.restarts++
		if s.cfg.MaxRetries > 0 && s.restarts > s.cfg.MaxRetries {
			logrus.Infof("[supervisor:%s] max retries reached", s.cfg.Name)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.RetryDelay):
		}
	}
}

func (s *Supervisor) runOnce(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, s.cfg.Cmd, s.cfg.Args...)
	cmd.Env = append(os.Environ(), s.cfg.Env...)
	cmd.Dir = s.cfg.Dir

	if s.cfg.Stdout != nil {
		cmd.Stdout = s.cfg.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if s.cfg.Stderr != nil {
		cmd.Stderr = s.cfg.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	// Linux: 让子进程成为独立进程组（便于 kill 整组）
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logrus.Infof("[supervisor:%s] starting: %s %v", s.cfg.Name, s.cfg.Cmd, s.cfg.Args)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", s.cfg.Name, err)
	}

	done := make(chan struct{})
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return context.Canceled
	}
	s.cmd = cmd
	s.done = done
	s.running = true
	s.mu.Unlock()

	err := cmd.Wait()

	s.mu.Lock()
	if s.cmd == cmd {
		s.cmd = nil
		s.done = nil
		s.running = false
		close(done)
	}
	s.mu.Unlock()
	return err
}

func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.stopping = true
	cmd, done := s.cmd, s.done
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}

	// 优雅退出
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	killGroup := err == nil && pgid > 0
	kill := func(sig syscall.Signal) {
		if killGroup {
			_ = syscall.Kill(-pgid, sig)
			return
		}
		_ = syscall.Kill(cmd.Process.Pid, sig)
	}

	kill(syscall.SIGTERM)

	timeout := s.cfg.StopTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
		kill(syscall.SIGKILL)
		// runOnce owns cmd.Wait. Give it a short bounded window to reap the
		// process after the hard kill, without calling Wait a second time.
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func (s *Supervisor) isStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}
