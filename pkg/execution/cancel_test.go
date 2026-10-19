package execution

import (
	"context"
	"testing"
	"time"
)

func TestCancellationKeepsFirstCauseAndDeadline(t *testing.T) {
	ctx, c := WithController(context.Background())
	c.SetTimeout(time.Second)
	c.Request("SIGTERM")
	first := c.Cause()
	c.SetTimeout(time.Hour)
	c.Request("SIGINT")
	if context.Cause(ctx) != first || c.Cause().Signal != "SIGTERM" || c.Cause().Deadline.Sub(first.At) != time.Second {
		t.Fatal("cancellation changed")
	}
	select {
	case <-c.Force():
	default:
		t.Fatal("second signal did not accelerate")
	}
}
func TestLateSignalDoesNotReplaceCommandResult(t *testing.T) {
	ctx, c := WithController(context.Background())
	c.Complete()
	c.Request("SIGINT")
	if context.Cause(ctx) != nil || c.Cause() != nil {
		t.Fatal("late signal changed result")
	}
	select {
	case <-c.Force():
	default:
		t.Fatal("late signal did not accelerate cleanup")
	}
}
