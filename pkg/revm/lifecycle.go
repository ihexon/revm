package revm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/execution"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

type Roots struct{ Runtime, Cache, State, Data string }

// ResolveRoots is shared by execution and management. XDG paths work on both
// supported hosts; the fallback runtime path is deliberately short and private.
func ResolveRoots() (Roots, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Roots{}, err
	}
	xdg := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return filepath.Join(v, "revm")
		}
		return filepath.Join(home, fallback, "revm")
	}
	r := Roots{Cache: xdg("XDG_CACHE_HOME", ".cache"), State: xdg("XDG_STATE_HOME", ".local/state"), Data: xdg("XDG_DATA_HOME", ".local/share"), Runtime: runtimeFallback()}
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" && len(filepath.Join(base, "revm")) <= 45 {
		r.Runtime = filepath.Join(base, "revm")
	}
	paths := []*string{&r.Runtime, &r.Cache, &r.State, &r.Data}
	for _, p := range paths {
		if !filepath.IsAbs(*p) {
			return r, fmt.Errorf("resource root must be absolute: %s", *p)
		}
		*p, err = canonicalPath(*p)
		if err != nil {
			return r, err
		}
	}
	for i, a := range paths {
		for j, b := range paths {
			if i < j && (pathWithin(*a, *b) || pathWithin(*b, *a)) {
				return r, fmt.Errorf("resource roots must not overlap: %s and %s", *a, *b)
			}
		}
	}
	return r, nil
}
func runtimeFallback() string { return fmt.Sprintf("/tmp/revm-%d", os.Getuid()) }
func canonicalPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(p)
	if parent == p {
		return "", err
	}
	parent, err = canonicalPath(parent)
	return filepath.Join(parent, filepath.Base(p)), err
}
func privateDir(p string) error {
	if err := os.MkdirAll(p, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("resource directory is not owned by this user: %s", p)
	}
	return os.Chmod(p, 0700)
}
func (r Roots) sessionDir(id string) string { return filepath.Join(r.State, "sessions", id) }
func (r Roots) sessionLock(id string) string {
	return filepath.Join(r.State, "locks", "session-"+id+".lock")
}
func (r Roots) volumeDir(id string) string { return filepath.Join(r.Data, "volumes", id) }
func (r Roots) volumeLock(id string) string {
	return filepath.Join(r.Data, "locks", "volume-"+id+".lock")
}

// Closing, rather than explicitly unlocking, retains the lock while an
// inherited VMM descriptor is alive. Stable lock files are never unlinked.
func acquireLock(path string) (*os.File, error) {
	if err := privateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("resource in use: %s: %w", path, err)
		}
		return nil, err
	}
	return f, nil
}
func activeLock(path string) (bool, error) {
	f, err := acquireLock(path)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	f.Close()
	return false, nil
}
func writeJSONAtomic(path string, value any) error {
	if err := privateDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".record-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

type Session struct {
	Version    int    `json:"version"`
	ID         string `json:"id"`
	Config     Config `json:"config"`
	VolumeID   string `json:"volumeID,omitempty"`
	InstanceID string `json:"instanceID,omitempty"`
}
type RunRecord struct {
	ExitCode   *int              `json:"exitCode,omitempty"`
	StateRoot  string            `json:"stateRoot"`
	Version    int               `json:"version"`
	SessionID  string            `json:"sessionID"`
	InstanceID string            `json:"instanceID"`
	RuntimeDir string            `json:"runtimeDir"`
	CacheDir   string            `json:"cacheDir"`
	StartedAt  time.Time         `json:"startedAt"`
	FinishedAt *time.Time        `json:"finishedAt,omitempty"`
	Status     string            `json:"status"`
	Result     *execution.Result `json:"result,omitempty"`
	Error      string            `json:"error,omitempty"`
}
type Volume struct {
	Version        int    `json:"version"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Purpose        string `json:"purpose"`
	Format         string `json:"format"`
	DefaultSession string `json:"defaultSession,omitempty"`
}
type sessionRun struct {
	roots      Roots
	session    Session
	record     RunRecord
	files      []*os.File
	logDir     string
	resultPath string
}

func loadSession(r Roots, id string) (Session, error) {
	var s Session
	if err := validateSessionID(id); err != nil {
		return s, err
	}
	if err := readJSON(filepath.Join(r.sessionDir(id), "session.json"), &s); err != nil {
		return s, fmt.Errorf("session %q: %w", id, err)
	}
	if s.Version != 1 || s.ID != id {
		return s, fmt.Errorf("unsupported or inconsistent session metadata for %q", id)
	}
	if s.InstanceID != "" {
		if _, err := uuid.Parse(s.InstanceID); err != nil {
			return s, fmt.Errorf("invalid instance ID in session %q", id)
		}
	}
	return s, nil
}
func loadRun(r Roots, s Session) (RunRecord, error) {
	var rec RunRecord
	err := readJSON(filepath.Join(r.sessionDir(s.ID), "runs", s.InstanceID, "result.json"), &rec)
	if err == nil && (rec.Version != 1 || rec.SessionID != s.ID || rec.InstanceID != s.InstanceID) {
		err = fmt.Errorf("inconsistent run metadata for %q", s.ID)
	}
	return rec, err
}
func loadVolume(r Roots, id string) (Volume, error) {
	var v Volume
	if _, err := uuid.Parse(id); err != nil {
		return v, fmt.Errorf("invalid volume ID: %s", id)
	}
	if err := readJSON(filepath.Join(r.volumeDir(id), "volume.json"), &v); err != nil {
		return v, fmt.Errorf("volume %s missing or unreadable (data will not be recreated): %w", id, err)
	}
	if v.Version != 1 || v.ID != id || v.Format != "ext4" {
		return v, fmt.Errorf("unsupported volume metadata: %s", id)
	}
	return v, nil
}
func volumes(r Roots) ([]Volume, error) {
	entries, err := os.ReadDir(filepath.Join(r.Data, "volumes"))
	if os.IsNotExist(err) {
		return []Volume{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Volume{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		v, err := loadVolume(r, e.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}
func beginRun(ctx context.Context, cfg *Config) (lease *sessionRun, retErr error) {
	r, err := ResolveRoots()
	if err != nil {
		return nil, err
	}
	lock, err := acquireLock(r.sessionLock(cfg.SessionID))
	if err != nil {
		return nil, err
	}
	l := &sessionRun{roots: r, files: []*os.File{lock}}
	var rollback []string
	defer func() {
		if retErr != nil {
			for i := len(rollback) - 1; i >= 0; i-- {
				retErr = errors.Join(retErr, os.RemoveAll(rollback[i]))
			}
			for _, f := range l.files {
				f.Close()
			}
		}
	}()
	if err := privateDir(filepath.Dir(r.sessionDir(cfg.SessionID))); err != nil {
		return nil, err
	}
	s, err := loadSession(r, cfg.SessionID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	newSession := errors.Is(err, os.ErrNotExist)
	if newSession {
		if _, statErr := os.Lstat(r.sessionDir(cfg.SessionID)); !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("session %q is incomplete; remove it with revm rm first", cfg.SessionID)
		}
		s = Session{Version: 1, ID: cfg.SessionID}
		if legacy, e := legacyResources(legacySessionDir(cfg.SessionID)); e == nil && legacy {
			var receipt migrationReceipt
			if readJSON(filepath.Join(r.State, "migrations", cfg.SessionID+".json"), &receipt) != nil || receipt.Version != 1 || receipt.Source != legacySessionDir(cfg.SessionID) {
				return nil, fmt.Errorf("legacy session %q exists; run revm migrate %s before starting it", cfg.SessionID, cfg.SessionID)
			}
			s.VolumeID = receipt.VolumeID
		} else if e != nil {
			return nil, e
		}
	}

	original := *cfg
	if cfg.RunMode == ModeContainer && cfg.ContainerDisk == nil {
		catalog, err := acquireLock(filepath.Join(r.Data, "locks", "catalog.lock"))
		if err != nil {
			return nil, err
		}
		v, err := ensureDefaultVolume(ctx, r, s)
		if err != nil {
			catalog.Close()
			return nil, err
		}
		s.VolumeID = v.ID
		lock, err := acquireLock(r.volumeLock(v.ID))
		catalog.Close()
		if err != nil {
			return nil, err
		}
		l.files = append(l.files, lock)
		cfg.ContainerDisk = &ContainerDiskSpec{Path: filepath.Join(r.volumeDir(v.ID), "disk.ext4")}
	}
	// External disk ownership never transfers to a session. A stable path lock
	// prevents two VMMs writing one disk, even across different state roots.
	diskPaths := []string{}
	for _, d := range cfg.Disks {
		diskPaths = append(diskPaths, d.Path)
	}
	if cfg.ContainerDisk != nil {
		diskPaths = append(diskPaths, cfg.ContainerDisk.Path)
	}
	seen := map[string]bool{}
	for _, p := range diskPaths {
		canonical, err := canonicalPath(p)
		if err != nil {
			return nil, err
		}
		if seen[canonical] {
			return nil, fmt.Errorf("disk attached more than once: %s", p)
		}
		seen[canonical] = true
		lock, err := lockDisk(canonical)
		if err != nil {
			return nil, err
		}
		l.files = append(l.files, lock)
	}
	// Build the first session privately. Publishing the directory is the only
	// point at which readers can observe it; later runs publish session.json last.
	sessionDir := r.sessionDir(s.ID)
	if newSession {
		if err := privateDir(filepath.Dir(sessionDir)); err != nil {
			return nil, err
		}
		sessionDir = filepath.Join(filepath.Dir(sessionDir), ".create-"+s.ID)
		// The session lock also owns this staging name. Recover an interrupted
		// initial publication without introducing a separate staging registry.
		if err = os.RemoveAll(sessionDir); err != nil {
			return nil, err
		}
		if err = os.Mkdir(sessionDir, 0700); err != nil {
			return nil, err
		}
		rollback = append(rollback, sessionDir)
	}
	instance := uuid.NewString()
	rec := RunRecord{StateRoot: r.State, Version: 1, SessionID: s.ID, InstanceID: instance, RuntimeDir: filepath.Join(r.Runtime, instance[:12]), CacheDir: filepath.Join(r.Cache, "instances", instance), StartedAt: time.Now().UTC(), Status: "starting"}
	if err = privateDir(r.Runtime); err != nil {
		return nil, err
	}
	for _, dir := range []string{rec.RuntimeDir, rec.CacheDir} {
		if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
			return nil, err
		}
		if err = os.Mkdir(dir, 0700); err != nil {
			return nil, err
		}
		rollback = append(rollback, dir)
		if err = writeJSONAtomic(filepath.Join(dir, "owner.json"), rec); err != nil {
			return nil, err
		}
	}
	runDir := filepath.Join(sessionDir, "runs", instance)
	rollback = append(rollback, runDir)
	l.logDir = filepath.Join(runDir, "logs")
	if err = privateDir(l.logDir); err != nil {
		return nil, err
	}
	s.Config = original
	s.InstanceID = instance
	l.session = s
	l.record = rec
	l.resultPath = filepath.Join(filepath.Dir(l.logDir), "result.json")
	if err = writeJSONAtomic(l.resultPath, rec); err != nil {
		return nil, err
	}
	if err = writeJSONAtomic(filepath.Join(sessionDir, "session.json"), s); err != nil {
		return nil, err
	}
	if newSession {
		if err = os.Rename(sessionDir, r.sessionDir(s.ID)); err != nil {
			return nil, err
		}
		rollback = append(rollback, r.sessionDir(s.ID))
		l.logDir = filepath.Join(r.sessionDir(s.ID), "runs", instance, "logs")
		l.resultPath = filepath.Join(filepath.Dir(l.logDir), "result.json")
		if err = syncDir(filepath.Dir(sessionDir)); err != nil {
			return nil, err
		}
	}
	cfg.paths = l
	return l, nil
}
func ensureDefaultVolume(ctx context.Context, r Roots, s Session) (Volume, error) {
	if s.VolumeID != "" {
		v, err := loadVolume(r, s.VolumeID)
		if err != nil {
			return v, err
		}
		_, err = os.Stat(filepath.Join(r.volumeDir(v.ID), "disk.ext4"))
		if err != nil {
			return v, fmt.Errorf("volume disk missing (will not recreate): %w", err)
		}
		return v, nil
	}
	list, err := volumes(r)
	if err != nil {
		return Volume{}, err
	}
	name := "podman-" + s.ID
	for _, v := range list {
		if v.DefaultSession == s.ID {
			if v.Name != name {
				return v, fmt.Errorf("default volume association conflict for %s", s.ID)
			}
			_, err = os.Stat(filepath.Join(r.volumeDir(v.ID), "disk.ext4"))
			if err != nil {
				return v, fmt.Errorf("volume disk missing (will not recreate): %w", err)
			}
			return v, nil
		}
		if v.Name == name {
			return v, fmt.Errorf("volume name %s is already associated with another session", name)
		}
	}
	v := Volume{Version: 1, ID: uuid.NewString(), Name: name, Purpose: "podman", Format: "ext4", DefaultSession: s.ID}
	base := filepath.Join(r.Data, "volumes")
	if err = privateDir(base); err != nil {
		return v, err
	}
	temp, err := os.MkdirTemp(base, ".create-")
	if err != nil {
		return v, err
	}
	defer os.RemoveAll(temp)
	spec, err := resolveContainerDiskSpec(nil, filepath.Join(temp, "disk.ext4"))
	if err != nil {
		return v, err
	}
	if _, err = createRAWDisk(ctx, spec); err != nil {
		return v, err
	}
	if err = writeJSONAtomic(filepath.Join(temp, "volume.json"), v); err != nil {
		return v, err
	}
	err = os.Rename(temp, r.volumeDir(v.ID))
	return v, err
}
func (l *sessionRun) finish(result *execution.Result, runErr error) error {
	now := time.Now().UTC()
	code := commanderr.ExitCode(runErr)
	l.record.ExitCode = &code
	l.record.FinishedAt = &now
	l.record.Status = "stopped"
	l.record.Result = result
	if runErr != nil {
		l.record.Error = runErr.Error()
		if result == nil {
			l.record.Status = "failed"
		}
	}
	return writeJSONAtomic(l.resultPath, l.record)
}
func removeOwned(dir, instance string) error {
	var owner RunRecord
	if err := readJSON(filepath.Join(dir, "owner.json"), &owner); err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Lstat(dir); os.IsNotExist(statErr) {
				return nil
			}
			return fmt.Errorf("resource directory has no ownership record; retained: %s", dir)
		}
		return err
	}
	if owner.Version != 1 || owner.InstanceID != instance {
		return fmt.Errorf("refusing to clean resources with different owner: %s", dir)
	}
	return os.RemoveAll(dir)
}
func (l *sessionRun) close(prior ...error) error {
	err := errors.Join(append(prior, removeOwned(l.record.RuntimeDir, l.record.InstanceID), removeOwned(l.record.CacheDir, l.record.InstanceID))...)
	// Commit cleanup failure before releasing the session lock, so wait cannot
	// observe a successful record for a CLI which is about to fail cleanup.
	if err != nil && l.record.FinishedAt != nil && !l.session.Config.Remove {
		if l.record.ExitCode == nil || *l.record.ExitCode == 0 {
			code := 1
			l.record.ExitCode = &code
		}
		if l.record.Result != nil {
			l.record.Result.Cleanup = "failed"
		}
		l.record.Error = commanderr.Diagnostic(errors.Join(errors.New(l.record.Error), err))
		err = errors.Join(err, writeJSONAtomic(l.resultPath, l.record))
	}
	for _, f := range l.files {
		err = errors.Join(err, f.Close())
	}
	l.files = nil
	return err
}

func legacySessionDir(id string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "revm", id)
}
func sessionWorkspace(id string) (string, error) {
	r, err := ResolveRoots()
	if err != nil {
		return "", err
	}
	s, err := loadSession(r, id)
	if err != nil {
		return "", err
	}
	active, err := activeLock(r.sessionLock(id))
	if err != nil {
		return "", err
	}
	if !active {
		return "", fmt.Errorf("session %q is stopped", id)
	}
	rec, err := loadRun(r, s)
	if err != nil {
		return "", err
	}
	return rec.RuntimeDir, nil
}

func legacyResources(dir string) (bool, error) {
	for _, name := range []string{"rootfs", "raw-disk", "logs", "socks", "ssh"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}
