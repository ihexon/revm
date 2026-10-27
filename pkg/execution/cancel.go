package execution

import (
	"context"
	"sync"
)

type Canceled struct{ Signal string }

func (c *Canceled) Error() string { return "canceled by " + c.Signal }
func (c *Canceled) ExitCode() int {
	if n := LinuxSignalNumber(c.Signal); n != 0 {
		return 128 + n
	}
	return 1
}

// Controller records the first cancellation and delivers terminal events.
type Controller struct {
	mu       sync.Mutex
	cancel   context.CancelCauseFunc
	cause    *Canceled
	terminal func(string)
}
type controllerKey struct{}

func WithController(ctx context.Context) (context.Context, *Controller) {
	ctx, cancel := context.WithCancelCause(ctx)
	c := &Controller{cancel: cancel}
	return context.WithValue(ctx, controllerKey{}, c), c
}
func ControllerFrom(ctx context.Context) *Controller {
	c, _ := ctx.Value(controllerKey{}).(*Controller)
	return c
}
func (c *Controller) Request(sig string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause != nil {
		return
	}
	c.cause = &Canceled{Signal: sig}
	c.cancel(c.cause)
}
func (c *Controller) Cause() *Canceled            { c.mu.Lock(); defer c.mu.Unlock(); return c.cause }
func (c *Controller) SetTerminal(fn func(string)) { c.mu.Lock(); defer c.mu.Unlock(); c.terminal = fn }
func (c *Controller) Terminal(sig string) {
	c.mu.Lock()
	fn := c.terminal
	c.mu.Unlock()
	if fn != nil {
		fn(sig)
	}
}
