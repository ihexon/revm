package revm

import (
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/network"
	"linuxvm/pkg/service/guestcontrol"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SessionView struct {
	Session
	Error     string            `json:"error,omitempty"`
	Status    string            `json:"status"`
	Active    bool              `json:"active"`
	LastRun   *RunRecord        `json:"lastRun,omitempty"`
	LogDir    string            `json:"logDir,omitempty"`
	Endpoints *SessionEndpoints `json:"endpoints,omitempty"`
}

type SessionEndpoints struct {
	Management string `json:"management"`
	Podman     string `json:"podman,omitempty"`
}

func inspectSession(r Roots, id string) (SessionView, error) {
	if err := validateSessionID(id); err != nil {
		return SessionView{}, err
	}
	info, err := os.Lstat(r.sessionDir(id))
	if err != nil {
		if os.IsNotExist(err) {
			return SessionView{}, fmt.Errorf("session %q not found: %w", id, os.ErrNotExist)
		}
		return SessionView{}, err
	}
	if !info.IsDir() {
		return SessionView{}, fmt.Errorf("session is not a directory: %s", id)
	}
	active, err := activeLock(r.sessionLock(id))
	if err != nil {
		return SessionView{}, err
	}
	s, err := loadSession(r, id)
	view := SessionView{Session: s, Active: active, Status: "stopped"}
	if err != nil {
		view.Session = Session{ID: id}
		view.Status, view.Error = "broken", err.Error()
		return view, nil
	}
	if s.InstanceID == "" {
		return view, nil
	}
	rec, err := loadRun(r, s)
	if err != nil {
		view.Status, view.Error = "broken", err.Error()
		return view, nil
	}
	view.LastRun = &rec
	view.Status = rec.Status
	view.LogDir = filepath.Join(r.sessionDir(id), "runs", s.InstanceID, "logs")
	if active {
		paths := newMachinePathManager(rec.RuntimeDir)
		address := func(path string) string { return (&url.URL{Scheme: "unix", Path: path}).String() }
		view.Endpoints = &SessionEndpoints{Management: address(paths.GetVMCtlSocketFile())}
		if s.Config.RunMode == ModeContainer {
			view.Endpoints.Podman = address(paths.GetPodmanSocketFile())
		}
	}
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
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		view, err := inspectSession(r, e.Name())
		if err != nil {
			view = SessionView{Session: Session{ID: e.Name()}, Status: "broken", Error: err.Error()}
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
	// Broken metadata must not prevent removing records. Scan only our known
	// disposable roots; never follow paths from a damaged result.json.
	if err := privateDir(filepath.Dir(r.sessionDir(id))); err != nil {
		return err
	}
	info, err := os.Lstat(r.sessionDir(id))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("session is not a directory: %s", id)
	}
	if err := privateDir(r.sessionDir(id)); err != nil {
		return err
	}
	for _, base := range []string{r.Runtime, filepath.Join(r.Cache, "instances")} {
		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(base, entry.Name())
			var owner RunRecord
			if readJSON(filepath.Join(dir, "owner.json"), &owner) != nil || owner.Version != 1 || owner.StateRoot != r.State || owner.SessionID != id {
				continue
			}
			if _, err := uuid.Parse(owner.InstanceID); err != nil {
				continue
			}
			expected := filepath.Join(r.Cache, "instances", owner.InstanceID)
			if base == r.Runtime {
				expected = filepath.Join(r.Runtime, owner.InstanceID[:12])
			}
			if dir != expected || (dir != owner.RuntimeDir && dir != owner.CacheDir) {
				continue
			}
			if err := removeOwned(dir, owner.InstanceID); err != nil {
				return err
			}
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
	for {
		_, status, requestErr := client.Post("/v2/stop").DoAndRead(ctx)
		if requestErr == nil {
			if status != 200 {
				return fmt.Errorf("stop API returned HTTP %d", status)
			}
			break
		}
		current, inspectErr := InspectSession(id)
		if inspectErr != nil {
			return inspectErr
		}
		if !current.Active || current.InstanceID != view.InstanceID {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("request stop: %w", context.Cause(ctx))
		case <-time.After(100 * time.Millisecond):
		}
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
	var lastProbe error
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
			lastProbe = err
		}
		select {
		case <-ctx.Done():
			if lastProbe != nil {
				return nil, fmt.Errorf("wait for session %q: %w (last readiness probe: %v)", s.ID, context.Cause(ctx), lastProbe)
			}
			return nil, context.Cause(ctx)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func SessionLogs(ctx context.Context, id, instance string, guest, follow bool, tail int, w io.Writer) error {
	if tail < -1 {
		return fmt.Errorf("log tail must be -1 (all lines) or nonnegative")
	}
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
		return fmt.Errorf("session has no run logs")
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
	if err := seekLogTail(file, tail); err != nil {
		return err
	}
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
