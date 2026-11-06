package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevmHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REVM_HOME", "")
	got, err := RevmHome()
	if err != nil || got != filepath.Join(home, ".revm") {
		t.Fatalf("default home = %q, %v", got, err)
	}
	custom := filepath.Join(home, "custom")
	t.Setenv("REVM_HOME", custom)
	t.Setenv("HOME", "")
	got, err = RevmHome()
	if err != nil || got != custom {
		t.Fatalf("custom home = %q, %v", got, err)
	}
	for _, invalid := range []string{"relative", "/"} {
		t.Setenv("REVM_HOME", invalid)
		if _, err := RevmHome(); err == nil {
			t.Fatalf("invalid home accepted: %s", invalid)
		}
	}
	alias := filepath.Join(home, "root-alias")
	if err := os.Symlink("/", alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVM_HOME", alias)
	if _, err := RevmHome(); err == nil {
		t.Fatal("filesystem root accepted through a symlink")
	}
}
