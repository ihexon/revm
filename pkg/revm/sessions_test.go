package revm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
	"linuxvm/pkg/commanderr"
	"linuxvm/pkg/execution"
)

func TestSessionWaitResultAndRemoval(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if err := RemoveSession(cfg.SessionID); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatal("removed active session", err)
	}
	code := 42
	result := execution.Result{Code: &code, Cleanup: "complete", Output: "complete"}
	if err := l.finish(&result, result.Err()); err != nil {
		t.Fatal(err)
	}
	l.close()
	if err := WaitSession(context.Background(), cfg.SessionID, false); commanderr.ExitCode(err) != 42 {
		t.Fatal("wait lost exit result", err)
	}
	view, err := InspectSession(cfg.SessionID)
	if err != nil || view.Active || view.Status != "stopped" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	if err := RemoveSession(cfg.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.sessionDir(cfg.SessionID)); !os.IsNotExist(err) {
		t.Fatal("session retained")
	}
	if _, err := os.Stat(r.sessionLock(cfg.SessionID)); err != nil {
		t.Fatal("stable lock removed")
	}
}
func TestCrashedSessionIsUnknown(t *testing.T) {
	testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	for _, f := range l.files {
		f.Close()
	}
	l.files = nil
	view, err := InspectSession(cfg.SessionID)
	if err != nil || view.Status != "unknown" {
		t.Fatalf("%+v %v", view, err)
	}
	if err := WaitSession(context.Background(), cfg.SessionID, false); err == nil {
		t.Fatal("unknown inferred as success")
	}
	if err := RemoveSession(cfg.SessionID); err != nil {
		t.Fatal(err)
	}
}
func TestRemoveSessionPreservesExternalData(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	external := filepath.Join(t.TempDir(), "external.ext4")
	os.WriteFile(external, []byte("keep"), 0600)
	cfg.Disks = []RawDiskSpec{{Path: external}}
	v := fakeVolume(t, r, cfg.SessionID)
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	l.close()
	if err := RemoveSession(cfg.SessionID); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{external, filepath.Join(r.volumeDir(v.ID), "disk.ext4")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("persistent data removed", p)
		}
	}
}

func TestWaitIncludesCleanupFailure(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(l.record.CacheDir)
	result := execution.FromProcess(nil)
	if err = l.finish(&result, nil); err != nil {
		t.Fatal(err)
	}
	owner := l.record
	owner.InstanceID = "different-owner"
	if err = writeJSONAtomic(filepath.Join(l.record.CacheDir, "owner.json"), owner); err != nil {
		t.Fatal(err)
	}
	if err = l.close(); err == nil {
		t.Fatal("cleanup ownership mismatch ignored")
	}
	if err = WaitSession(context.Background(), cfg.SessionID, false); commanderr.ExitCode(err) != 1 {
		t.Fatal("cleanup failure recorded as success", err)
	}
	view, err := inspectSession(r, cfg.SessionID)
	if err != nil || view.LastRun.Result.Cleanup != "failed" {
		t.Fatalf("%+v %v", view, err)
	}
}
