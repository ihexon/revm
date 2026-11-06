package execution

import (
	"context"
	"errors"
	"io"
	"syscall"
)

// CancelOnBrokenPipe marks EPIPE from a command's output sink as cancellation.
// It also wakes the CLI boundary immediately, even if another output writer is
// blocked. Internal transports must handle their own errors, not use this wrapper.
// The executable is responsible for disabling default SIGPIPE termination.
func CancelOnBrokenPipe(ctx context.Context, dst io.Writer) io.Writer {
	return outputWriter{dst: dst, controller: ControllerFrom(ctx)}
}

type outputWriter struct {
	dst        io.Writer
	controller *Controller
}

func (w outputWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if errors.Is(err, syscall.EPIPE) {
		if w.controller != nil {
			w.controller.Request("SIGPIPE")
		}
		err = errors.Join(&Canceled{Signal: "SIGPIPE"}, err)
	}
	return n, err
}
