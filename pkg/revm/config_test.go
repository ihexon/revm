package revm

import (
	"linuxvm/pkg/define"
	"strings"
	"testing"

	sysproxy "github.com/ihexon/getSysProxy"
)

func TestSystemProxyReachesExecutionRequest(t *testing.T) {
	previous := getSystemHTTPProxy
	t.Cleanup(func() { getSystemHTTPProxy = previous })
	getSystemHTTPProxy = func() (*sysproxy.Item, error) {
		return &sysproxy.Item{Scheme: "http", Host: "127.0.0.1", Port: 8080}, nil
	}
	for _, mode := range []RunMode{ModeRootfs, ModeContainer} {
		cfg := testConfig()
		cfg.RunMode, cfg.Proxy = mode, true
		cfg.Exec.Env = []string{"https_proxy=http://explicit:9000"}
		resolved, err := NormalizeConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		env := map[string]string{}
		for _, value := range resolved.Exec.Env {
			key, value, ok := strings.Cut(value, "=")
			if ok {
				env[key] = value
			}
		}
		if env["http_proxy"] != "http://host.containers.internal:8080" || env["https_proxy"] != "http://explicit:9000" {
			t.Fatalf("proxy lost or explicit env overridden: %v", env)
		}
	}
}

func TestRawDiskSyncNoneSurvivesPreparation(t *testing.T) {
	for _, tc := range []struct {
		option string
		want   uint32
	}{{"", define.SyncModeRelaxed}, {",sync=none", define.SyncModeNone}, {",sync=full", define.SyncModeFull}} {
		spec, err := ParseRawDiskSpec("/tmp/disk.ext4" + tc.option)
		if err != nil {
			t.Fatal(err)
		}
		dev := applyRawDiskRuntimeOptions(define.BlockDeviceSpec{}, spec)
		if dev.SyncMode != tc.want {
			t.Fatalf("%s: sync=%d want %d", tc.option, dev.SyncMode, tc.want)
		}
	}
}
