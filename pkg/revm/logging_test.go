package revm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupDiagnosticIncludesBoundedLogAndSession(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("earlier output\n", 500) + "fatal: cannot mount rootfs\n"
	if err := os.WriteFile(filepath.Join(dir, "vm.log"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	lease := &sessionRun{logDir: dir, session: Session{ID: "failed-run"}}
	cause := errors.New("VMM exited with code 1")
	err := lease.diagnose(cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "fatal: cannot mount rootfs") || !strings.Contains(err.Error(), "revm inspect failed-run") || len(err.Error()) > 2500 {
		t.Fatal(err)
	}
	if strings.Count(guestLogTail(filepath.Join(dir, "vm.log")), "\n") > 7 {
		t.Fatal("unbounded log tail")
	}
}
