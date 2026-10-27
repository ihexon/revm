package guestcontrol

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitReadyReportsLastFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := WaitReady(ctx, "/tmp/revm-missing-readiness-test.sock")
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "last probe:") || !strings.Contains(err.Error(), "revm-missing-readiness-test.sock") {
		t.Fatal(err)
	}
}
