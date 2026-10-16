// Package podmanproxy carries the native Podman byte stream over a private
// vsock channel. Yamux supplies half-close and flow control independently of
// libkrun's Unix socket bridge; tcpproxy handles bidirectional copying.
package podmanproxy

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/inetaf/tcpproxy"
	yamux "github.com/libp2p/go-yamux/v5"
)

type dialFunc func(context.Context) (net.Conn, error)

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

func serve(ctx context.Context, listener net.Listener, dial dialFunc) error {
	return serveConnections(ctx, listener, func(ctx context.Context, conn net.Conn) {
		var stopBackend func() bool
		defer func() {
			if stopBackend != nil {
				stopBackend()
			}
		}()
		proxy := &tcpproxy.DialProxy{DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
			dialCtx, cancel := context.WithCancel(dialCtx)
			stopDial := context.AfterFunc(ctx, cancel)
			defer stopDial()
			defer cancel()
			backend, err := dial(dialCtx)
			if err == nil {
				stopBackend = context.AfterFunc(ctx, func() { _ = backend.Close() })
			}
			return backend, err
		}}
		proxy.HandleConn(conn)
	})
}

// ServeHost exposes the Podman Unix socket to native clients. All connections
// share a Yamux session, which is recreated on the next dial if disconnected.
func ServeHost(ctx context.Context, socket, transportSocket string) error {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return err
	}
	client := &client{socket: transportSocket}
	defer client.Close()
	return serve(ctx, listener, client.Open)
}

type client struct {
	mu      sync.Mutex
	socket  string
	session *yamux.Session
}

func (c *client) Open(ctx context.Context) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil || c.session.IsClosed() {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socket)
		if err != nil {
			return nil, err
		}
		c.session, err = yamux.Client(conn, nil, nil)
		if err != nil {
			conn.Close()
			return nil, err
		}
	}
	return c.session.Open(ctx)
}

func (c *client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		_ = c.session.Close()
	}
}

// ServeGuest accepts only connections already authenticated by the vsock
// listener, and connects their streams to Podman's private Unix socket.
func ServeGuest(ctx context.Context, listener net.Listener, socket string) error {
	return serveConnections(ctx, listener, func(ctx context.Context, conn net.Conn) {
		session, err := yamux.Server(conn, nil, nil)
		if err != nil {
			return
		}
		defer session.Close()
		_ = serve(ctx, session, func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		})
	})
}
