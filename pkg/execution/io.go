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
			return 0, err
		}
		defer unix.Close(fd)
		src = &pollReader{ctx: ctx, fd: fd}
	}
	return io.Copy(dst, src)
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

// outputWriters snapshots both descriptors before changing their shared flags
// (stdout and stderr may refer to the same open file description).
func outputWriters(ctx context.Context, writers ...io.Writer) ([]io.Writer, func(), error) {
	type saved struct{ fd, flags int }
	var files []saved
	restore := func() {
		for _, f := range files {
			_, _ = unix.FcntlInt(uintptr(f.fd), unix.F_SETFL, f.flags)
			_ = unix.Close(f.fd)
		}
	}
	result := append([]io.Writer(nil), writers...)
	for i, dst := range writers {
		if dst == nil {
			result[i] = io.Discard
			continue
		}
		if f, ok := dst.(*os.File); ok {
			fd, err := unix.Dup(int(f.Fd()))
			if err != nil {
				restore()
				return nil, nil, err
			}
			flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
			if err != nil {
				unix.Close(fd)
				restore()
				return nil, nil, err
			}
			files = append(files, saved{fd, flags})
			result[i] = &pollWriter{ctx: ctx, fd: fd}
		}
	}
	for _, f := range files {
		if err := unix.SetNonblock(f.fd, true); err != nil {
			restore()
			return nil, nil, err
		}
	}
	return result, restore, nil
}

type pollWriter struct {
	ctx context.Context
	fd  int
}

func (w *pollWriter) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		if err := w.ctx.Err(); err != nil {
			return total, err
		}
		fds := []unix.PollFd{{Fd: int32(w.fd), Events: unix.POLLOUT}}
		n, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return total, err
		}
		if n == 0 {
			continue
		}
		n, err = unix.Write(w.fd, b[:min(len(b), 4096)])
		if n > 0 {
			total += n
			b = b[n:]
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
