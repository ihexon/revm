package test_suit

import (
	"linuxvm/pkg/system"
	"testing"
)

func TestRlimit(t *testing.T) {
	err := system.Rlimit()
	if err != nil {
		t.Fatalf("%v", err)
	}
}
