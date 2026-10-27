package revm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"linuxvm/pkg/system"
)

// The write end stays open for the entire CLI process, including native code.
// Only process death closes it; closing it on a Go return would clean a live VM.
var cleanupOwner atomic.Pointer[os.File]

type cleanupState struct {
	Roots    Roots
	Session  Session
	Record   RunRecord
	Terminal *unix.Termios
}

// TerminalRestored is called by main immediately before exit. A killed or
// native-exited parent cannot send this, so the helper restores its terminal.
func TerminalRestored() {
	if owner := cleanupOwner.Load(); owner != nil {
		_ = json.NewEncoder(owner).Encode(true)
	}
}

func startCleanup(ctx context.Context, l *sessionRun, cancel context.CancelCauseFunc) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if launcher := os.Getenv("REVM_BUNDLE_LAUNCHER"); launcher != "" && runtime.GOOS == "linux" {
		executable = launcher
	}
	life, owner, err := os.Pipe()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(l.logDir, "revm.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		life.Close()
		owner.Close()
		return err
	}
	defer log.Close()
	cmd := exec.Command(executable, "_cleanup")
	cmd.ExtraFiles = append([]*os.File{life, os.Stdin}, l.files...)
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = cmd.Start()
	life.Close()
	if err != nil {
		owner.Close()
		return err
	}
	// Keep all inherited locks until this process has died and cleanup finishes.
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = errors.New("cleanup helper exited before VM process")
		}
		cancel(fmt.Errorf("cleanup helper: %w", err))
	}()
	state := cleanupState{Roots: l.roots, Session: l.session, Record: l.record, Terminal: system.SaveTerminal(int(os.Stdin.Fd()))}
	started := make(chan error, 1)
	go func() {
		started <- json.NewEncoder(owner).Encode(state)
	}()
	select {
	case err = <-started:
	case <-ctx.Done():
		err = context.Cause(ctx)
	}
	if err != nil {
		return fmt.Errorf("start cleanup helper: %w", err)
	}
	cleanupOwner.Store(owner)
	return nil
}

// RunCleanup owns no VM services and never inherits command output. It can
// finish after SIGKILL without keeping a pipeline open or consuming more stdin.
func RunCleanup() error {
	life := os.NewFile(3, "owner-lifetime")
	terminal := os.NewFile(4, "owner-terminal")
	defer life.Close()
	defer terminal.Close()
	decoder := json.NewDecoder(life)
	var state cleanupState
	if err := decoder.Decode(&state); err != nil {
		return err
	}
	restored := false
	for {
		var value bool
		if decoder.Decode(&value) != nil {
			break
		}
		restored = restored || value
	}
	if !restored {
		if err := system.RestoreTerminal(int(terminal.Fd()), state.Terminal); err != nil {
			fmt.Fprintln(os.Stderr, "restore terminal:", err)
		}
	}
	terminal.Close()
	l := &sessionRun{roots: state.Roots, session: state.Session, record: state.Record}
	l.logDir = filepath.Join(l.roots.sessionDir(l.session.ID), "runs", l.record.InstanceID, "logs")
	l.resultPath = filepath.Join(filepath.Dir(l.logDir), "result.json")
	rec, err := loadRun(l.roots, l.session)
	if err == nil {
		l.record = rec
	}
	if l.record.FinishedAt == nil {
		now := time.Now().UTC()
		l.record.FinishedAt = &now
		l.record.Status = "unknown"
		l.record.Error = "VM process exited without a recorded command result"
		err = errors.Join(err, writeJSONAtomic(l.resultPath, l.record))
	}
	paths := newMachinePathManager(l.record.RuntimeDir)
	if link := l.session.Config.PodmanProxyAPIFile; link != "" {
		if actual, e := os.Readlink(link); e == nil && actual == paths.GetPodmanSocketFile() {
			err = errors.Join(err, os.Remove(link))
		}
	}
	return l.close(err)
}
