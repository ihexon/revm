package revm

import (
	"bytes"
	"context"
	"errors"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
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
func testConfig() VMOptions {
	return VMOptions{RunMode: define.RootFsMode, SessionID: "test-" + uuid.NewString(), Network: "gvisor", ExecOptions: ExecOptions{Exec: execution.Request{Argv: []string{"true"}}}}
}
func fakeVolume(t *testing.T, r Roots, id string) Volume {
	t.Helper()
	v := Volume{Version: 1, ID: uuid.NewString(), Name: "podman-" + id, Purpose: "podman", Format: "ext4"}
	if err := writeJSONAtomic(filepath.Join(r.volumeDir(v.Name), "volume.json"), v); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.volumeDir(v.Name), "disk.ext4"), []byte("persistent data"), 0600); err != nil {
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
	cfg.RunMode = define.ContainerMode
	cfg.VolumeName = "podman-" + cfg.SessionID
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
	// The same explicit volume can be reused by a different session.
	os.RemoveAll(r.sessionDir(cfg.SessionID))
	cfg.SessionID = "another-session"
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
	cfg.ContainerDisk = nil
	if _, err := beginRun(context.Background(), &cfg); err == nil {
		t.Fatal("missing volume reference silently recreated")
	}
}

func TestDamagedVolumeDoesNotBlockOtherVolumes(t *testing.T) {
	r := testRoots(t)
	broken := fakeVolume(t, r, "broken")
	if err := os.WriteFile(filepath.Join(r.volumeDir(broken.Name), "volume.json"), []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	// Creation and attachment must not read unrelated metadata.
	good, err := CreateVolume(context.Background(), "good")
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.RunMode, cfg.VolumeName = define.ContainerMode, good.Name
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	views, err := ListVolumes()
	if err != nil || len(views) != 2 {
		t.Fatal(views, err)
	}
	if views[0].Name != "good" || !views[0].InUse || views[0].Error != "" || views[1].Error == "" {
		t.Fatalf("%+v", views)
	}
	if err := RemoveVolume(good.Name); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatal("active volume deletion", err)
	}
	l.close()
	if err := RemoveVolume(good.Name); err != nil {
		t.Fatal(err)
	}
	// Broken metadata must neither authorize replacement nor prevent explicit deletion.
	if _, err := CreateVolume(context.Background(), broken.Name); err == nil {
		t.Fatal("replaced a broken volume")
	}
	if err := RemoveVolume(broken.Name); err != nil {
		t.Fatal(err)
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
func TestExistingDisksAreNeverReplaced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data.ext4")
	original := []byte("existing data, not an ext4 disk")
	if err := os.WriteFile(p, original, 0600); err != nil {
		t.Fatal(err)
	}
	b := newMachineBuilder(define.RootFsMode)
	if _, err := b.prepareRawDisk(context.Background(), RawDiskSpec{Path: p}); err == nil {
		t.Fatal("invalid raw disk accepted")
	}
	if _, err := b.prepareContainerStorageDisk(context.Background(), &ContainerDiskSpec{Path: p}, ""); err == nil {
		t.Fatal("invalid container disk accepted")
	}
	got, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("existing disk changed", err)
	}
}
