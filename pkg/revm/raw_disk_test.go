//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"linuxvm/pkg/define"
	"testing"
)

func TestParseRawDiskStorageOptions(t *testing.T) {
	spec, err := ParseRawDiskSpec("/tmp/disk.raw,readonly=true,directio=true,sync=full,mnt=/data")
	if err != nil {
		t.Fatal(err)
	}
	if !spec.ReadOnly || !spec.DirectIO || spec.SyncMode != define.SyncModeFull || spec.MountTo != "/data" {
		t.Fatalf("unexpected storage spec: %+v", spec)
	}
}

func TestParseRawDiskRejectsInvalidSyncMode(t *testing.T) {
	if _, err := ParseRawDiskSpec("/tmp/disk.raw,sync=bad"); err == nil {
		t.Fatal("expected invalid sync mode error")
	}
}
