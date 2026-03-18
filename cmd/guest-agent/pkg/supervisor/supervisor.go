package supervisor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	cmd *exec.Cmd

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
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.runOnce(ctx)
		if err != nil {
			logrus.Infof("[supervisor:%s] process exited: %s", s.cfg.Name, err)
		}

		if !s.cfg.Restart {
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

	s.cmd = cmd
	s.running = true

	err := cmd.Wait()

	s.running = false
	return err
}

func (s *Supervisor) Stop() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	// 优雅退出
	pgid, _ := syscall.Getpgid(s.cmd.Process.Pid)

	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-done:
		return
	case <-time.After(s.cfg.StopTimeout):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
}
