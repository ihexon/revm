package libkrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// ProcessProvider contains native VMM process exits and supplies an actual
// force-stop boundary even when libkrun's blocking C call cannot return.
type ProcessProvider struct {
	cmd     *exec.Cmd
	control *os.File
	mu      sync.Mutex
	done    chan struct{}
	err     error
	forced  bool
	started bool
}

func NewProcessProvider(ctx context.Context, spec *define.MachineSpec) (*ProcessProvider, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	status, statusWriter, err := os.Pipe()
	if err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}
	cmd := exec.Command(exe, "_vmm")
	cmd.ExtraFiles = []*os.File{reader, statusWriter}
	logFile, err := os.OpenFile(spec.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		reader.Close()
		writer.Close()
		status.Close()
		statusWriter.Close()
		return nil, err
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		status.Close()
		statusWriter.Close()
		return nil, err
	}
	reader.Close()
	statusWriter.Close()
	p := &ProcessProvider{cmd: cmd, control: writer, done: make(chan struct{})}
	go func() { err := cmd.Wait(); p.mu.Lock(); p.err = err; p.mu.Unlock(); close(p.done) }()
	ready := make(chan error, 1)
	go func() {
		defer status.Close()
		if err := json.NewEncoder(writer).Encode(spec); err != nil {
			ready <- err
			return
		}
		var failure string
		err := json.NewDecoder(status).Decode(&failure)
		if err == nil && failure != "" {
			err = errors.New(failure)
		}
		ready <- err
	}()
	select {
	case err = <-ready:
	case <-ctx.Done():
		err = context.Cause(ctx)
	case <-time.After(15 * time.Second):
		err = errors.New("VMM creation timed out")
	}
	if err != nil {
		status.Close()
		writer.Close()
		_ = cmd.Process.Kill()
		<-p.done
		return nil, err
	}
	return p, nil
}

func (p *ProcessProvider) Start(context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return errors.New("VMM already started")
	}
	p.started = true
	err := json.NewEncoder(p.control).Encode("start")
	p.mu.Unlock()
	if err != nil {
		return err
	}
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	// Guest command status comes from the result protocol. A native process exit
	// code is not a substitute for that result (including native exit(0)).
	var exit *exec.ExitError
	if errors.As(p.err, &exit) && !exit.ProcessState.Exited() && !p.forced {
		return fmt.Errorf("VMM terminated: %w", p.err)
	}
	return nil
}

func (p *ProcessProvider) RequestShutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
		return nil
	default:
	}
	_ = p.control.SetWriteDeadline(time.Now().Add(time.Second))
	defer p.control.SetWriteDeadline(time.Time{})
	return json.NewEncoder(p.control).Encode("shutdown")
}

func (p *ProcessProvider) ForceStop(ctx context.Context) error {
	p.mu.Lock()
	p.forced = true
	p.mu.Unlock()
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (p *ProcessProvider) Close() error {
	_ = p.control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return p.ForceStop(ctx)
}

// RunChild is only called by the executable's private _vmm entry point. The
// inherited pipe owns this process: losing the parent returns to main even if
// the native VMM thread is stuck. No library calls os.Exit.
func RunChild() error {
	control := os.NewFile(3, "vmm-control")
	status := os.NewFile(4, "vmm-status")
	defer control.Close()
	defer status.Close()
	dec := json.NewDecoder(control)
	var spec define.MachineSpec
	if err := dec.Decode(&spec); err != nil {
		return err
	}
	if file, err := os.OpenFile(spec.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		defer file.Close()
		logrus.SetOutput(file)
	}
	// Start reading before native initialization: it may block inside C. EOF
	// must still return from this private entry point and terminate the child.
	commands := make(chan string)
	go func() {
		defer close(commands)
		for {
			var command string
			if dec.Decode(&command) != nil {
				return
			}
			commands <- command
		}
	}()
	type creation struct {
		provider *Provider
		err      error
	}
	created := make(chan creation, 1)
	go func() {
		p, err := NewProvider(context.Background(), &spec)
		created <- creation{p, err}
	}()
	var p *Provider
	var err error
	select {
	case result := <-created:
		p, err = result.provider, result.err
	case <-commands:
		return errors.New("VMM owner disconnected during initialization")
	}
	failure := ""
	if err != nil {
		failure = err.Error()
	}
	if sendErr := json.NewEncoder(status).Encode(failure); sendErr != nil {
		return sendErr
	}
	if err != nil {
		return err
	}
	first := <-commands
	if first != "start" {
		return errors.New("expected VMM start")
	}
	done := make(chan error, 1)
	go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); done <- p.Start(context.Background()) }()
	for {
		select {
		case err := <-done:
			return errors.Join(err, p.Close())
		case command, ok := <-commands:
			if !ok {
				return errors.New("VMM owner disconnected")
			}
			if command == "shutdown" {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_ = p.RequestShutdown(ctx)
				}()
			}
		}
	}
}
