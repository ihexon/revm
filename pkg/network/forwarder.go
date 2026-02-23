package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"time"
)

type LocalForwarder struct {
	UnixSockAddr string
	Target       string
	Timeout      time.Duration
}

func (s *LocalForwarder) Run(ctx context.Context) error {
	parse, err := url.Parse(s.UnixSockAddr)
	if err != nil {
		return err
	}

	_ = os.Remove(parse.Path)

	l, err := net.Listen("unix", parse.Path)
	if err != nil {
		return err
	}

	if err := os.Chmod(parse.Path, 0600); err != nil {
		return err
	}

	defer l.Close()

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *LocalForwarder) handleConn(ctx context.Context, uconn net.Conn) {
	defer uconn.Close()

	dialer := net.Dialer{
		Timeout: s.Timeout,
	}

	tconn, err := dialer.DialContext(ctx, "tcp", s.Target)
	if err != nil {
		return
	}
	defer tconn.Close()

	proxy(ctx, uconn, tconn)
}

func proxy(ctx context.Context, a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyConn := func(dst, src net.Conn) {
		defer wg.Done()

		_, _ = io.Copy(dst, src)

		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		if unix, ok := dst.(*net.UnixConn); ok {
			_ = unix.CloseWrite()
		}
	}

	go copyConn(b, a)
	go copyConn(a, b)

	go func() {
		<-ctx.Done()
		a.Close()
		b.Close()
	}()

	wg.Wait()
}
