package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCancellationRunsServiceCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := New(Config{Name: "test", Cmd: "sh", Args: []string{"-c", `trap 'echo stopped > "$1/stopped"; exit 0' TERM; echo ready > "$1/ready"; while :; do sleep 0.1; done`, "test", dir}, StopTimeout: time.Second})
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("service did not stop")
	}
	if _, err := os.Stat(filepath.Join(dir, "stopped")); err != nil {
		t.Fatal("service was killed before TERM cleanup", err)
	}
}

func TestServiceExitIsFailure(t *testing.T) {
	if err := New(Config{Name: "test", Cmd: "true"}).Run(context.Background()); err == nil {
		t.Fatal("service exit treated as success")
	}
}

func TestCancellationKillsStubbornService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := New(Config{Name: "test", Cmd: "sh", Args: []string{"-c", "trap '' TERM; while :; do sleep 1; done"}, StopTimeout: 100 * time.Millisecond}).Run(ctx)
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("cancel failed: %v", err)
	}
}
