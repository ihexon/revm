package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentDependencyCache(t *testing.T) {
	source := t.TempDir()
	payload := filepath.Join(source, "payload")
	if err := os.WriteFile(payload, []byte("immutable dependency"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "fixture.tar.zst")
	if err := command(nil, "bsdtar", "--zstd", "-cf", archive, "-C", source, "."); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	cache := t.TempDir()
	b := &builder{cacheDir: cache, archiveDir: filepath.Join(cache, "archives"), deps: depsLock{BaseURL: server.URL, Assets: map[string]string{"fixture.tar.zst": fmt.Sprintf("%x", sha256.Sum256(data))}}}
	if err := os.MkdirAll(b.archiveDir, 0755); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	paths := make(chan string, 4)
	for range 4 {
		wg.Go(func() {
			path, err := b.archivePath("fixture.tar.zst")
			if err != nil {
				t.Error(err)
				return
			}
			extracted, err := b.extractArchive(path)
			if err != nil {
				t.Error(err)
				return
			}
			paths <- extracted
		})
	}
	wg.Wait()
	close(paths)
	var result string
	for path := range paths {
		if result != "" && result != path {
			t.Fatal("same dependency published to different objects")
		}
		result = path
	}
	if result == "" {
		t.Fatal("dependency was not published")
	}
	original, err := os.Stat(filepath.Join(result, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.extractArchive(archive); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(filepath.Join(result, "payload"))
	if err != nil || !os.SameFile(original, after) {
		t.Fatal("cache hit replaced live dependency files")
	}
	got, err := os.ReadFile(filepath.Join(result, "payload"))
	if err != nil || string(got) != "immutable dependency" {
		t.Fatalf("partial extraction published: %q %v", got, err)
	}
	// Another version of the same asset name must not replace the old object.
	other := &builder{cacheDir: cache, archiveDir: b.archiveDir, deps: depsLock{BaseURL: server.URL, Assets: map[string]string{"fixture.tar.zst": strings.Repeat("0", 64)}}}
	if _, err := other.archivePath("fixture.tar.zst"); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	got, err = os.ReadFile(filepath.Join(result, "payload"))
	if err != nil || string(got) != "immutable dependency" {
		t.Fatal("failed update damaged existing cache")
	}
}

func TestFailedBuildPreservesSourceAndOutput(t *testing.T) {
	root := t.TempDir()
	assets := map[string]string{
		"pkg/static_resources/rootfs/rootfs.tar.zst":   "custom rootfs",
		"pkg/static_resources/guest_agent/guest-agent": "custom guest agent",
		"out/revm/bin/revm":                            "last usable build",
	}
	for name, data := range assets {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cache := t.TempDir()
	b := &builder{sourceDir: root, goos: "darwin", goarch: "arm64", targets: []string{"revm"}, cacheDir: cache, archiveDir: filepath.Join(cache, "archives"), deps: depsLock{Assets: map[string]string{}}}
	if err := b.run(); err == nil {
		t.Fatal("missing dependency accepted")
	}
	for name, want := range assets {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatalf("build modified %s: %q %v", name, data, err)
		}
	}
	leftovers, err := filepath.Glob(filepath.Join(root, "out", ".build-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("staging leak: %v %v", leftovers, err)
	}
}

func TestConcurrentBuildSameOutputRejected(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "out")
	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(output, ".build.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	b := &builder{sourceDir: root}
	if err := b.run(); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("concurrent publisher accepted: %v", err)
	}
}
