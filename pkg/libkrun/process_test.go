package libkrun

import (
	"context"
	"encoding/json"
	"errors"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "_vmm" {
		dec := json.NewDecoder(os.NewFile(3, "control"))
		var spec define.MachineSpec
		if dec.Decode(&spec) != nil {
			os.Exit(99)
		}
		json.NewEncoder(os.NewFile(4, "status")).Encode("")
		var start string
		if dec.Decode(&start) != nil || start != "start" {
			os.Exit(98)
		}
		mode := os.Getenv("REVM_TEST_VMM_EXIT")
		if mode == "wait" {
			for {
				time.Sleep(time.Hour)
			}
		}
		code, err := strconv.Atoi(mode)
		if err != nil {
			os.Exit(97)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func testProcess(t *testing.T, mode string) *ProcessProvider {
	t.Helper()
	t.Setenv("REVM_TEST_VMM_EXIT", mode)
	t.Setenv("REVM_BUNDLE_LAUNCHER", "")
	p, err := NewProcessProvider(context.Background(), &define.MachineSpec{LogFile: filepath.Join(t.TempDir(), "vmm.log")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestVMMExitStatus(t *testing.T) {
	for _, code := range []int{0, 42} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			p := testProcess(t, strconv.Itoa(code))
			err := p.Start(context.Background())
			if code == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != code || !strings.Contains(err.Error(), "VMM terminated") {
				t.Fatalf("lost VMM failure: %v", err)
			}
			p.ForceStop(context.Background())
			if p.forced {
				t.Fatal("already exited VMM marked as force stopped")
			}
		})
	}
}

func TestVMMForceStop(t *testing.T) {
	p := testProcess(t, "wait")
	started := make(chan error, 1)
	go func() { started <- p.Start(context.Background()) }()
	// Wait until Start has sent the request before terminating the child.
	for {
		p.mu.Lock()
		ok := p.started
		p.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := p.ForceStop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-started; err != nil {
		t.Fatal(err)
	}
}
