package backend

import (
	"context"
)

type Backend interface {
	// Start blocks until the VMM exits. Cancelling ctx must not return early and
	// must never race Close against krun_vmm_run.
	Start(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	RequestShutdown(ctx context.Context) error
	ForceStop(ctx context.Context) error
	Close() error
}
