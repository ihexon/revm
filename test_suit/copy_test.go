package test_suit

import (
	"linuxvm/pkg/system"
	"testing"
)

func TestCopy(t *testing.T) {
	err := system.CopyDHClientInToRootFS("/tmp/")
	if err != nil {
		t.Fatalf("%v", err)
	}
}
