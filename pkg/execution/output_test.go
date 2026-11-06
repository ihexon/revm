package execution

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
)

func TestBrokenOutputCancelsAtWrite(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()
	ctx, _ := WithController(context.Background())
	n, err := CancelOnBrokenPipe(ctx, w).Write([]byte("output"))
	var canceled *Canceled
	if n != 0 || !errors.Is(err, syscall.EPIPE) || !errors.As(err, &canceled) || canceled.ExitCode() != 141 {
		t.Fatalf("write returned %d, %v", n, err)
	}
	if cause := context.Cause(ctx); !errors.As(cause, &canceled) || canceled.ExitCode() != 141 {
		t.Fatalf("write did not immediately cancel the CLI: %v", cause)
	}
}

func TestOutputFailureClassification(t *testing.T) {
	for _, failure := range []error{syscall.EPIPE, syscall.ENOSPC, io.ErrClosedPipe} {
		t.Run(failure.Error(), func(t *testing.T) {
			// Library callers need the typed error even without a CLI controller.
			n, err := CancelOnBrokenPipe(context.Background(), partialWriter{failure}).Write([]byte("output"))
			var canceled *Canceled
			if n != 2 || !errors.Is(err, failure) || errors.As(err, &canceled) != errors.Is(failure, syscall.EPIPE) {
				t.Fatalf("write returned %d, %v", n, err)
			}
		})
	}
}

type partialWriter struct{ err error }

func (w partialWriter) Write([]byte) (int, error) { return 2, w.err }
