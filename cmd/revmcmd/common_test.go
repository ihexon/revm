package revmcmd

import (
	"bytes"
	"context"
	"linuxvm/pkg/commanderr"
	"testing"
)

func TestInvalidInvocations(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"}, {"--unknown"}, {"run"},
		{"run", "--id", "../escape", "--", "true"},
		{"run", "--id", "test", "--log-level", "bogus", "--", "true"},
		{"run", "--id", "test", "--memory", "1", "--", "true"},
		{"run", "--id", "test", "--network", "bogus", "--", "true"},
		{"run", "--id", "test", "--envs", "INVALID", "--", "true"},
		{"ctl", "--id", "test"},
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
