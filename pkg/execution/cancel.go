package execution

import (
	"context"
	"sync"
	"time"
)

type Canceled struct {
	Signal   string
	At       time.Time
	Deadline time.Time
}

func (c *Canceled) Error() string { return "canceled by " + c.Signal }
func (c *Canceled) ExitCode() int {
	if n := LinuxSignalNumber(c.Signal); n != 0 {
		return 128 + n
	}
	return 1
}

// Controller fixes the deadline on the first signal. Repeated signals only
// accelerate cleanup. Completing a command prevents late signals changing it.
type Controller struct {
	mu          sync.Mutex
	cancel      context.CancelCauseFunc
	timeout     time.Duration
	cause       *Canceled
	commandDone bool
	forced      bool
	force       chan struct{}
	terminal    func(string)
}
type controllerKey struct{}

func WithController(ctx context.Context) (context.Context, *Controller) {
	ctx, cancel := context.WithCancelCause(ctx)
	c := &Controller{cancel: cancel, timeout: 10 * time.Second, force: make(chan struct{})}
	return context.WithValue(ctx, controllerKey{}, c), c
}
func ControllerFrom(ctx context.Context) *Controller {
	c, _ := ctx.Value(controllerKey{}).(*Controller)
	return c
}
func (c *Controller) SetTimeout(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.timeout = d }
func (c *Controller) Request(sig string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause != nil || c.commandDone {
		if !c.forced {
			close(c.force)
			c.forced = true
		}
		return
	}
	now := time.Now()
	c.cause = &Canceled{Signal: sig, At: now, Deadline: now.Add(c.timeout)}
	c.cancel(c.cause)
}
func (c *Controller) Cause() *Canceled            { c.mu.Lock(); defer c.mu.Unlock(); return c.cause }
func (c *Controller) Complete()                   { c.mu.Lock(); defer c.mu.Unlock(); c.commandDone = true }
func (c *Controller) Force() <-chan struct{}      { return c.force }
func (c *Controller) SetTerminal(fn func(string)) { c.mu.Lock(); defer c.mu.Unlock(); c.terminal = fn }
func (c *Controller) Terminal(sig string) {
	c.mu.Lock()
	fn := c.terminal
	c.mu.Unlock()
	if fn != nil {
		fn(sig)
	}
}
