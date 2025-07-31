package system

import (
	"linuxvm/pkg/system"
	"testing"
)

func TestGetOSInfo(t *testing.T) {
	info, err := system.GetOSVersion()
	if err != nil {
		t.Fatalf("system.GetOSVersion err: %v", err)
	}
	t.Logf("info: %+v", info)
}
