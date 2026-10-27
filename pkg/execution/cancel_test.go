package execution

import (
	"context"
	"testing"
)

func TestCancellationKeepsFirstCause(t *testing.T) {
	ctx, c := WithController(context.Background())
	c.Request("SIGTERM")
	first := c.Cause()
	c.Request("SIGINT")
	if context.Cause(ctx) != first || c.Cause().Signal != "SIGTERM" {
		t.Fatal("cancellation changed")
	}
	if ctx.Err() == nil {
		t.Fatal("signal did not cancel immediately")
	}
}
