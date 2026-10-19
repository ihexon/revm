package revm

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestBrokenSessionCanBeListedAndRemoved(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	l.close()
	broken := r.sessionDir("broken")
	if err := os.MkdirAll(filepath.Join(broken, "runs", "incomplete"), 0700); err != nil {
		t.Fatal(err)
	}
	volume := fakeVolume(t, r, "broken")
	views, err := ListSessions()
	if err != nil || len(views) != 2 {
		t.Fatalf("%+v %v", views, err)
	}
	view, err := InspectSession("broken")
	if err != nil || view.Status != "broken" || view.Error == "" {
		t.Fatalf("%+v %v", view, err)
	}
	if err := RemoveSession("broken"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.volumeDir(volume.ID), "disk.ext4")); err != nil {
		t.Fatal("removed persistent volume", err)
	}
}

func TestBrokenRunRemovalDoesNotTrustRecordPaths(t *testing.T) {
	testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range l.files {
		f.Close()
	}
	l.files = nil // Simulate owner death.
	outside := t.TempDir()
	malicious := l.record
	malicious.RuntimeDir = outside
	if err := writeJSONAtomic(filepath.Join(outside, "owner.json"), malicious); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(l.resultPath, malicious); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSession(cfg.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("followed untrusted result path")
	}
	for _, path := range []string{l.record.RuntimeDir, l.record.CacheDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("owned disposable resource retained", path, err)
		}
	}
}

func TestCreationFailureRollsBackUnpublishedResources(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	if err := os.MkdirAll(r.Cache, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Cache, "instances"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := beginRun(context.Background(), &cfg); err == nil {
		t.Fatal("expected creation failure")
	}
	entries, err := os.ReadDir(filepath.Join(r.State, "sessions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial session remains: %v %v", entries, err)
	}
	runtime, err := os.ReadDir(r.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range runtime {
		var owner RunRecord
		if readJSON(filepath.Join(r.Runtime, entry.Name(), "owner.json"), &owner) == nil && owner.SessionID == cfg.SessionID {
			t.Fatal("runtime leaked")
		}
	}
	if err := os.Remove(filepath.Join(r.Cache, "instances")); err != nil {
		t.Fatal(err)
	}
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal("cannot retry failed creation", err)
	}
	l.close()
}

func TestTemporaryRunRetention(t *testing.T) {
	for _, mode := range []string{"success", "guest", "signal", "startup", "cleanup", "record", "crash"} {
		t.Run(mode, func(t *testing.T) {
			r := testRoots(t)
			cfg := testConfig()
			cfg.Remove = true
			l, err := beginRun(context.Background(), &cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(l.record.CacheDir)
			if err := os.WriteFile(filepath.Join(l.logDir, "vm.log"), []byte("boot failed\n"), 0600); err != nil {
				t.Fatal(err)
			}
			result := execution.FromProcess(nil)
			var runErr, recordErr error
			switch mode {
			case "guest":
				code := 42
				result.Code = &code
				runErr = result.Err()
			case "signal":
				runErr = &execution.Canceled{Signal: "SIGTERM"}
			case "startup":
				runErr = errors.New("VMM failed")
			}
			if mode != "crash" {
				if err := l.finish(&result, runErr); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "record" {
				recordErr = errors.New("cannot save result")
			}
			if mode == "cleanup" {
				owner := l.record
				owner.InstanceID = "different-owner"
				if err := writeJSONAtomic(filepath.Join(l.record.CacheDir, "owner.json"), owner); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "crash" {
				for _, f := range l.files {
					f.Close()
				}
				l.files = nil
			} else {
				err = l.close(recordErr)
				if (err != nil) != (mode == "cleanup" || mode == "record") {
					t.Fatal(err)
				}
			}
			if _, err := PruneCache(); err != nil {
				t.Fatal(err)
			}
			if mode == "success" {
				if _, err := os.Stat(r.sessionDir(cfg.SessionID)); !os.IsNotExist(err) {
					t.Fatal("successful temporary session retained", err)
				}
				return
			}
			view, err := InspectSession(cfg.SessionID)
			if err != nil || view.Active {
				t.Fatalf("lost failure records: %+v %v", view, err)
			}
			if _, err := os.Stat(filepath.Join(l.logDir, "vm.log")); err != nil {
				t.Fatal("lost log", err)
			}
			if _, err := os.Stat(l.record.RuntimeDir); !os.IsNotExist(err) {
				t.Fatal("runtime retained", err)
			}
			if mode != "cleanup" {
				if _, err := os.Stat(l.record.CacheDir); !os.IsNotExist(err) {
					t.Fatal("cache retained", err)
				}
			}
			if mode == "guest" && *view.LastRun.ExitCode != 42 {
				t.Fatal("lost guest status")
			}
			if mode == "signal" && *view.LastRun.ExitCode != 143 {
				t.Fatal("lost signal status")
			}
			if err := RemoveSession(cfg.SessionID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInspectEndpointsAndVolumeUsage(t *testing.T) {
	r := testRoots(t)
	cfg := testConfig()
	cfg.RunMode = ModeContainer
	volume := fakeVolume(t, r, cfg.SessionID)
	disk := filepath.Join(r.volumeDir(volume.ID), "disk.ext4")
	if err := os.Truncate(disk, 8*1024*1024); err != nil {
		t.Fatal(err)
	}
	cfg.VolumeName = volume.Name
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	view, err := InspectSession(cfg.SessionID)
	if err != nil || view.Endpoints == nil || !strings.HasSuffix(view.Endpoints.Podman, "/socks/podman-api.sock") || view.LogDir != l.logDir {
		t.Fatalf("%+v %v", view, err)
	}
	volumes, err := ListVolumes()
	if err != nil || len(volumes) != 1 {
		t.Fatal(volumes, err)
	}
	v := volumes[0]
	if v.Path != disk || !v.InUse || v.SizeBytes == nil || *v.SizeBytes != 8*1024*1024 || v.AllocatedBytes == nil || *v.AllocatedBytes >= *v.SizeBytes {
		t.Fatalf("%+v", v)
	}
	if err := l.close(); err != nil {
		t.Fatal(err)
	}
	view, err = InspectSession(cfg.SessionID)
	if err != nil || view.Endpoints != nil {
		t.Fatal("stopped endpoint advertised", view, err)
	}
	if err := os.Remove(disk); err != nil {
		t.Fatal(err)
	}
	volumes, err = ListVolumes()
	if err != nil || len(volumes) != 1 || volumes[0].Error == "" || volumes[0].SizeBytes != nil {
		t.Fatal("missing disk hidden", volumes, err)
	}
}

func TestLogTailFollowsNewLines(t *testing.T) {
	testRoots(t)
	cfg := testConfig()
	l, err := beginRun(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	path := filepath.Join(l.logDir, "revm.log")
	if err := os.WriteFile(path, []byte("old\nlast\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w := &notifyingBuffer{ready: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- SessionLogs(ctx, cfg.SessionID, "", false, true, 1, w) }()
	select {
	case <-w.ready:
	case <-ctx.Done():
		t.Fatal("log reader did not start")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("new\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = l.close(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if w.String() != "last\nnew\n" {
		t.Fatalf("%q", w.String())
	}
}

type notifyingBuffer struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (w *notifyingBuffer) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	w.once.Do(func() { close(w.ready) })
	return n, err
}

func (w *notifyingBuffer) String() string { return w.buffer.String() }
