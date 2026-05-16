package backend

import (
	"context"
)

type Backend interface {
	// vmWaitAbortCtx only aborts the host-side wait for the VM to exit. It must
	// not be used as the graceful guest shutdown request path.
	Start(vmWaitAbortCtx context.Context) error
	RequestShutdown(ctx context.Context) error
	ForceStop(ctx context.Context) error
}
