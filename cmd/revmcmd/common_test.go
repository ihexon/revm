package revmcmd

import (
	"bytes"
	"context"
	"linuxvm/pkg/commanderr"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestInvalidInvocations(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"attach"}, {"migrate"}, {"exec"}, {"podman"},
		{"podman", "--volume", "data", "--container-disk", "/tmp/disk"},
		{"run", "--rm", "--", "true"},
		{"volume", "create", "../escape"}, {"--unknown"}, {"run"},
		{"run", "--id", "../escape", "--", "true"},
		{"run", "--id", ".create-session", "--", "true"},
		{"run", "--id", "bad-cpu", "--cpus", "-1", "--", "true"},
		{"run", "--id", "test", "--log-level", "bogus", "--", "true"},
		{"run", "--id", "test", "--memory", "1", "--", "true"},
		{"run", "--network", "invalid", "--", "true"},
		{"run", "--network", "tsi", "--port-export", "8080:80", "--", "true"},
		{"run", "--system-proxy", "--", "true"},
		{"run", "--manage-api", "/tmp/revm.sock", "--", "true"},
		{"run", "--id", "test", "--envs", "INVALID", "--", "true"},
		{"ctl", "--id", "test"}, {"ports"}, {"ports", "a", "b"},
		{"run", "--port-export", "80", "--", "true"},
		{"exec", "--id", "test", "--envs", "INVALID"},
		{"volume", "unknown"}, {"cache", "unknown"}, {"ls", "unexpected"},
		{"inspect"}, {"rm", "a", "b"}, {"stop", "--timeout=-1s", "test"},
		{"wait", "../escape"}, {"logs", "../escape"},
		{"run", "--id", "test", "--tty=wrong", "--", "true"},
		{"run", "--id", "test", "--pty", "--tty=never", "--", "true"},
	} {
		t.Run(args[0], func(t *testing.T) {
			app := NewApp("revm")
			var out, diagnostic bytes.Buffer
			app.Writer, app.ErrWriter = &out, &diagnostic
			err := app.Run(context.Background(), append([]string{"revm"}, args...))
			if code := commanderr.ExitCode(err); code != 2 {
				t.Fatalf("%v: code=%d err=%v", args, code, err)
			}
			if out.Len() != 0 {
				t.Fatalf("usage failure polluted stdout: %q", out.String())
			}
		})
	}
}

func TestHelpSucceeds(t *testing.T) {
	app := NewApp("revm")
	var out bytes.Buffer
	app.Writer = &out
	if err := app.Run(context.Background(), []string{"revm", "run", "--help"}); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("missing help")
	}
}

func TestReleaseInterfaces(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"completion", "bash"}, {"completion", "zsh"}, {"completion", "fish"}, {"_complete", "run"}} {
		app := NewApp("revm")
		var out bytes.Buffer
		app.Writer = &out
		if err := app.Run(context.Background(), append([]string{"revm"}, args...)); err != nil {
			t.Fatal(args, err)
		}
		if out.Len() == 0 {
			t.Fatal("missing release output", args)
		}
	}
	for _, args := range [][]string{{"completion"}, {"completion", "unknown"}, {"completion", "bash", "extra"}} {
		app := NewApp("revm")
		var out bytes.Buffer
		app.Writer = &out
		err := app.Run(context.Background(), append([]string{"revm"}, args...))
		if commanderr.ExitCode(err) != 2 {
			t.Fatal(args, err)
		}
	}
}

func TestHelpDocumentsRequiredArgumentsAndSupportedShells(t *testing.T) {
	for _, name := range []string{"inspect", "stop", "wait", "logs", "rm", "ports"} {
		app := NewApp("revm")
		var out bytes.Buffer
		app.Writer = &out
		if err := app.Run(context.Background(), []string{"revm", name, "--help"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "SESSION") || !strings.Contains(out.String(), "Example") {
			t.Fatalf("incomplete help: %s", out.String())
		}
	}
	app := NewApp("revm")
	var out bytes.Buffer
	app.Writer = &out
	if err := app.Run(context.Background(), []string{"revm", "completion", "--help"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out.String()), "powershell") || !strings.Contains(out.String(), "SHELL") {
		t.Fatal(out.String())
	}
}

func TestResourceNameCompletion(t *testing.T) {
	root := t.TempDir()
	for key, sub := range map[string]string{"XDG_STATE_HOME": "state", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "XDG_RUNTIME_DIR": "runtime"} {
		t.Setenv(key, filepath.Join(root, sub))
	}
	session := filepath.Join(root, "state/revm/sessions/dev")
	volume := filepath.Join(root, "data/revm/volumes/data")
	for _, dir := range []string{session, volume} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(session, "session.json"), []byte(`{"version":1,"id":"dev","config":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "volume.json"), []byte(`{"version":1,"id":"4fe61727-80d4-4aef-a132-d04527086cdd","name":"data","format":"ext4"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "disk.ext4"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"logs"}, "dev\n"}, {[]string{"logs", "--guest"}, "--guest\ndev\n"},
		{[]string{"exec", "--id"}, "dev\n"}, {[]string{"ports"}, "dev\n"},
		{[]string{"podman", "--volume"}, "data\n"}, {[]string{"volume", "rm"}, "data\n"},
		{[]string{"run", "--tt"}, "--tty\n"},
	} {
		app := NewApp("revm")
		var out bytes.Buffer
		app.Writer = &out
		args := append([]string{"revm", "_complete"}, tc.args...)
		if err := app.Run(context.Background(), args); err != nil {
			t.Fatal(args, err)
		}
		if out.String() != tc.want {
			t.Fatalf("%v: got %q, want %q", args, out.String(), tc.want)
		}
	}
	// A completion request after -- must not launch the VM or create a session.
	if err := Execute(context.Background(), []string{"revm", "_complete", "run", "--id", "completion-must-not-run", "--", "false"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "state/revm/sessions/completion-must-not-run")); !os.IsNotExist(err) {
		t.Fatal("completion created a session", err)
	}
}

func TestCompletionFlagIsAnOrdinaryGuestArgument(t *testing.T) {
	want := []string{"printf", "%s", "--generate-shell-completion"}
	app := NewApp("revm")
	var got []string
	app.Command("run").Action = func(_ context.Context, c *cli.Command) error {
		got = c.Args().Slice()
		return nil
	}
	if err := app.Run(context.Background(), append([]string{"revm", "run", "--"}, want...)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("guest arguments changed: got %q, want %q", got, want)
	}
}
