//go:build linux || darwin

package execution

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// CopyContext polls file input so a terminal read can be stopped without
// closing the caller's stdin or leaving a goroutine consuming future input.
func CopyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if file, ok := src.(*os.File); ok {
		fd, err := unix.Dup(int(file.Fd()))
		if err != nil {
			return 0, &inputReadError{err}
		}
		defer unix.Close(fd)
		src = &pollReader{ctx: ctx, fd: fd}
	}
	return io.Copy(dst, &checkedReader{src})
}

// Only source failures are fatal input errors. A command may legitimately stop
// consuming stdin early; its closed input stream is not a host read failure.
type inputReadError struct{ error }

func (e *inputReadError) Unwrap() error { return e.error }

type checkedReader struct{ io.Reader }

func (r *checkedReader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	if err != nil && err != io.EOF {
		err = &inputReadError{err}
	}
	return n, err
}

type pollReader struct {
	ctx context.Context
	fd  int
}

func (r *pollReader) Read(b []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		fds := []unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		n, err = unix.Read(r.fd, b)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		return n, err
	}
}
