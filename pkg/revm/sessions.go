package revm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/guestcontrol"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type SessionView struct {
	Session
	Status  string     `json:"status"`
	Active  bool       `json:"active"`
	LastRun *RunRecord `json:"lastRun,omitempty"`
}

func inspectSession(r Roots, id string) (SessionView, error) {
	s, err := loadSession(r, id)
	if err != nil {
		return SessionView{}, err
	}
	active, err := activeLock(r.sessionLock(id))
	if err != nil {
		return SessionView{}, err
	}
	view := SessionView{Session: s, Active: active, Status: "stopped"}
	if s.InstanceID == "" {
		return view, nil
	}
	rec, err := loadRun(r, s)
	if err != nil {
		return view, err
	}
	view.LastRun = &rec
	view.Status = rec.Status
	if !active && rec.FinishedAt == nil {
		view.Status = "unknown"
		view.LastRun.Status = "unknown"
	}
	return view, nil
}
func InspectSession(id string) (SessionView, error) {
	r, err := ResolveRoots()
	if err != nil {
		return SessionView{}, err
	}
	return inspectSession(r, id)
}
func ListSessions() ([]SessionView, error) {
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(r.State, "sessions"))
	if os.IsNotExist(err) {
		return []SessionView{}, nil
	}
	if err != nil {
		return nil, err
	}
	views := []SessionView{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		view, err := inspectSession(r, e.Name())
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}
func RemoveSession(id string) error {
	r, err := ResolveRoots()
	if err != nil {
		return err
	}
	if err := validateSessionID(id); err != nil {
		return err
	}
	lock, err := acquireLock(r.sessionLock(id))
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := loadSession(r, id); err != nil {
		return err
	}
	runs, err := os.ReadDir(filepath.Join(r.sessionDir(id), "runs"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range runs {
		if !entry.IsDir() {
			continue
		}
		if _, err := uuid.Parse(entry.Name()); err != nil {
			return fmt.Errorf("unrecognized run record %s; retained", entry.Name())
		}
		rec, err := loadRun(r, Session{ID: id, InstanceID: entry.Name()})
		if err != nil {
			return err
		}
		if rec.StateRoot != r.State {
			return fmt.Errorf("run belongs to another state root; retained")
		}
		if err = errors.Join(removeOwned(rec.RuntimeDir, rec.InstanceID), removeOwned(rec.CacheDir, rec.InstanceID)); err != nil {
			return err
		}
	}
	return os.RemoveAll(r.sessionDir(id))
}
func StopSession(ctx context.Context, id string) error {
	view, err := InspectSession(id)
	if err != nil {
		return err
	}
	if !view.Active {
		return nil
	}
	if view.LastRun == nil {
		return fmt.Errorf("session %q has no instance to stop", id)
	}
	client := network.NewUnixClient(newMachinePathManager(view.LastRun.RuntimeDir).GetVMCtlSocketFile())
	defer client.Close()
	_, status, err := client.Post("/v2/stop").DoAndRead(ctx)
	if err != nil {
		return fmt.Errorf("request stop: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("stop API returned HTTP %d", status)
	}
	_, err = waitInstance(ctx, view.Session, false, true)
	return err
}
func WaitSession(ctx context.Context, id string, ready bool) error {
	r, err := ResolveRoots()
	if err != nil {
		return err
	}
	s, err := loadSession(r, id)
	if err != nil {
		return err
	}
	result, err := waitInstance(ctx, s, ready, false)
	if err != nil {
		return err
	}
	if result != nil && result.ExitCode != nil {
		code := *result.ExitCode
		return (execution.Result{Code: &code, Cleanup: "complete", Output: "complete"}).Err()
	}
	return nil
}
func waitInstance(ctx context.Context, s Session, ready, stop bool) (*RunRecord, error) {
	if s.InstanceID == "" {
		return nil, fmt.Errorf("session %q has not run", s.ID)
	}
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	for {
		rec, err := loadRun(r, s)
		if err != nil {
			return nil, err
		}
		current, err := loadSession(r, s.ID)
		if err != nil {
			return nil, err
		}
		active, err := activeLock(r.sessionLock(s.ID))
		if err != nil {
			return nil, err
		}
		finished := !active || current.InstanceID != s.InstanceID
		if finished {
			if ready {
				return &rec, fmt.Errorf("instance stopped before becoming ready")
			}
			if stop {
				return &rec, nil
			}
			if rec.FinishedAt == nil || rec.ExitCode == nil {
				return &rec, fmt.Errorf("instance exited without a recorded result; status unknown")
			}
			return &rec, nil
		}
		if ready {
			probe, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
			_, err := guestcontrol.WaitReady(probe, guestcontrol.Target{UnixSocket: newMachinePathManager(rec.RuntimeDir).GetGuestControlSocketFile()})
			cancel()
			if err == nil {
				return &rec, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func SessionLogs(ctx context.Context, id, instance string, guest, follow bool, w io.Writer) error {
	r, err := ResolveRoots()
	if err != nil {
		return err
	}
	s, err := loadSession(r, id)
	if err != nil {
		return err
	}
	if instance != "" {
		if _, err := uuid.Parse(instance); err != nil {
			return fmt.Errorf("invalid instance ID")
		}
		s.InstanceID = instance
	}
	if s.InstanceID == "" {
		return fmt.Errorf("session has no run logs; migrated logs are in %s", filepath.Join(r.sessionDir(id), "legacy-logs"))
	}
	if _, err = loadRun(r, s); err != nil {
		return err
	}
	name := "revm.log"
	if guest {
		name = "vm.log"
	}
	file, err := os.Open(filepath.Join(r.sessionDir(id), "runs", s.InstanceID, "logs", name))
	if err != nil {
		return err
	}
	defer file.Close()
	for {
		if _, err = io.Copy(w, file); err != nil {
			return err
		}
		if !follow {
			return nil
		}
		current, err := loadSession(r, id)
		if err != nil {
			return err
		}
		active, err := activeLock(r.sessionLock(id))
		if err != nil {
			return err
		}
		if !active || current.InstanceID != s.InstanceID {
			_, err = io.Copy(w, file)
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
