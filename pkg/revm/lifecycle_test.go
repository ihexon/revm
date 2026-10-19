package revm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

func testRoots(t *testing.T) Roots {
	t.Helper()
	base := t.TempDir()
	for key, sub := range map[string]string{"XDG_CACHE_HOME": "cache", "XDG_STATE_HOME": "state", "XDG_DATA_HOME": "data", "XDG_RUNTIME_DIR": "run"} {
		t.Setenv(key, filepath.Join(base, sub))
	}
	r, err := ResolveRoots()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func testConfig() Config {
	c := *DefaultConfig()
	c.RunMode = ModeRootfs
	c.SessionID = "test-" + uuid.NewString()
	c.Exec.Argv = []string{"true"}
	return c
}
func fakeVolume(t *testing.T, r Roots, id string) Volume {
	t.Helper()
	v := Volume{Version: 1, ID: uuid.NewString(), Name: "podman-" + id, DefaultSession: id, Purpose: "podman", Format: "ext4"}
	if err := writeJSONAtomic(filepath.Join(r.volumeDir(v.ID), "volume.json"), v); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.volumeDir(v.ID), "disk.ext4"), []byte("persistent data"), 0600); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestResourceRootsRejectOverlap(t *testing.T) {
	testRoots(t)
	t.Setenv("XDG_DATA_HOME", os.Getenv("XDG_CACHE_HOME"))
	if _, err := ResolveRoots(); err == nil {
		t.Fatal("overlapping roots accepted")
	}
}
func TestSessionResourcesAndStableLock(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	lockInfo, _ := os.Stat(r.sessionLock(cfg.SessionID))
	if _, err := beginRun(context.Background(), &cfg); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("duplicate run: %v", err)
	}
	log := filepath.Join(l.logDir, "vm.log")
	os.WriteFile(log, []byte("log"), 0600)
	if err := l.finish(nil, errors.New("test failure")); err != nil {
		t.Fatal(err)
	}
	first := l.record.InstanceID
	if err := l.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.record.RuntimeDir); !os.IsNotExist(err) {
		t.Fatal("runtime was not removed")
	}
	if _, err := os.Stat(log); err != nil {
		t.Fatal("log was removed", err)
	}
	cfg.paths = nil
	l2, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.close()
	if first == l2.record.InstanceID {
		t.Fatal("instance reused")
	}
	now, _ := os.Stat(r.sessionLock(cfg.SessionID))
	if !os.SameFile(lockInfo, now) {
		t.Fatal("lock inode changed")
	}
}
func TestPersistentVolumeReuseAndMissingReference(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	cfg.RunMode = ModeContainer
	v := fakeVolume(t, r, cfg.SessionID)
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if l.session.VolumeID != v.ID {
		t.Fatal("original volume not reused")
	}
	if err := RemoveVolume(v.Name); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("active volume deletion: %v", err)
	}
	l.close()
	// Removing session metadata must not lose the volume's default association.
	os.RemoveAll(r.sessionDir(cfg.SessionID))
	cfg.paths = nil
	cfg.ContainerDisk = nil
	l2, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.close()
	if l2.session.VolumeID != v.ID {
		t.Fatal("session recreation lost volume")
	}
	l2.close()
	if err := RemoveVolume(v.Name); err != nil {
		t.Fatal(err)
	}
	cfg.paths = nil
	cfg.ContainerDisk = nil
	if _, err := beginRun(context.Background(), &cfg); err == nil {
		t.Fatal("missing volume reference silently recreated")
	}
}
func TestPrunePreservesActiveForeignAndLegacy(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	legacy := filepath.Join(r.Cache, "legacy", "raw-disk")
	os.MkdirAll(legacy, 0700)
	os.WriteFile(filepath.Join(legacy, "disk"), []byte("keep"), 0600)
	if _, err := PruneCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.record.CacheDir); err != nil {
		t.Fatal("pruned active instance")
	}
	foreign := filepath.Join(r.Runtime, "test-foreign-"+uuid.NewString()[:8])
	os.MkdirAll(foreign, 0700)
	defer os.RemoveAll(foreign)
	owner := l.record
	owner.RuntimeDir = foreign
	owner.StateRoot = "/another/state"
	writeJSONAtomic(filepath.Join(foreign, "owner.json"), owner)
	// Simulate owner death, retaining its resource records for the pruner.
	for _, f := range l.files {
		f.Close()
	}
	l.files = nil
	if _, err := PruneCache(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{legacy, foreign} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("pruned data not owned by this state root", p)
		}
	}
	if _, err := os.Stat(l.record.CacheDir); !os.IsNotExist(err) {
		t.Fatal("stale instance not pruned")
	}
}
func TestInheritedLockSurvivesOwnerClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stable.lock")
	f, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "10")
	cmd.ExtraFiles = []*os.File{f}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	f.Close()
	if lock, err := acquireLock(path); err == nil {
		lock.Close()
		t.Fatal("inherited lock released while child alive")
	}
	cmd.Process.Kill()
	cmd.Wait()
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
}
func TestDiskVersionNeverAuthorizesReplacement(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.ext4")
	original := []byte("existing data, not an ext4 disk")
	os.WriteFile(p, original, 0600)
	if err := newRawDiskXattrManager().SetXattr(context.Background(), p, "user.vm.rawdisk.version", "v1", true); err != nil {
		t.Fatal(err)
	}
	b := newMachineBuilder(0)
	if _, err := b.prepareRawDisk(context.Background(), RawDiskSpec{Path: p, Version: "v2"}); err == nil {
		t.Fatal("version mismatch accepted")
	}
	if _, err := b.prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{Path: p, Version: "v2"}, ""); err == nil {
		t.Fatal("container version mismatch accepted")
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, original) {
		t.Fatal("existing disk changed")
	}
	unknown := filepath.Join(t.TempDir(), "unknown.ext4")
	os.WriteFile(unknown, original, 0600)
	if _, err := b.prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{Path: unknown}, ""); err == nil {
		t.Fatal("invalid disk accepted")
	}
	got, _ = os.ReadFile(unknown)
	if !bytes.Equal(got, original) {
		t.Fatal("disk with missing version was replaced")
	}
}
