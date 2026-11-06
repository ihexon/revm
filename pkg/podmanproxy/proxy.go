// Package podmanproxy carries the native Podman byte stream over a private
// vsock channel. Each client owns one transport. Bounded data frames carry EOF
// without half-closing libkrun's Unix socket bridge.
package podmanproxy

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type halfCloseConn interface {
	net.Conn
	CloseWrite() error
}

type dialFunc func(context.Context) (halfCloseConn, error)

// serve owns the listener and all accepted connections until it returns.
func serveConnections(ctx context.Context, listener net.Listener, handle func(context.Context, net.Conn)) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	var peers sync.WaitGroup
	defer func() {
		cancel()
		_ = listener.Close()
		stop()
		peers.Wait()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		peers.Go(func() {
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			defer conn.Close()
			handle(ctx, conn)
		})
	}
}

// proxyConnection owns both endpoints. EOF preserves the response direction;
// any copy error or service cancellation closes both ends and joins the copies.
func proxyConnection(ctx context.Context, conn halfCloseConn, dial dialFunc) {
	defer conn.Close()
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	backend, err := dial(dialCtx)
	cancel()
	if err != nil {
		return
	}
	defer backend.Close()
	closeBoth := func() { conn.Close(); backend.Close() }
	stop := context.AfterFunc(ctx, closeBoth)
	defer stop()
	done := make(chan error, 2)
	copyHalf := func(dst, src halfCloseConn) {
		_, err := io.Copy(dst, src)
		if err == nil {
			err = dst.CloseWrite()
		}
		done <- err
	}
	go copyHalf(conn, backend)
	go copyHalf(backend, conn)
	if err := <-done; err != nil {
		closeBoth()
	}
	<-done
}

// ServeHost gives each native client its own guest transport.
func ServeHost(ctx context.Context, socket, transportSocket string) error {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return err
	}
	return serveConnections(ctx, listener, func(ctx context.Context, conn net.Conn) {
		proxyConnection(ctx, conn.(*net.UnixConn), func(ctx context.Context) (halfCloseConn, error) {
			transport, err := dialUnix(ctx, transportSocket)
			if err != nil {
				return nil, err
			}
			return &framedConn{Conn: transport}, nil
		})
	})
}

func dialUnix(ctx context.Context, socket string) (*net.UnixConn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, err
	}
	return conn.(*net.UnixConn), nil
}

// ServeGuest accepts only connections already authenticated by the vsock
// listener. Each transport carries exactly one Podman connection.
func ServeGuest(ctx context.Context, listener net.Listener, socket string) error {
	return serveConnections(ctx, listener, func(ctx context.Context, conn net.Conn) {
		proxyConnection(ctx, &framedConn{Conn: conn}, func(ctx context.Context) (halfCloseConn, error) {
			return dialUnix(ctx, socket)
		})
	})
}
